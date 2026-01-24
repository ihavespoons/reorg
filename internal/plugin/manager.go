// Package plugin provides plugin management functionality for the reorg host.
package plugin

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/hashicorp/go-hclog"
	goplugin "github.com/hashicorp/go-plugin"

	"github.com/ihavespoons/reorg/internal/llm"
	"github.com/ihavespoons/reorg/internal/service"
	"github.com/ihavespoons/reorg/pkg/plugin"
)

// Manager handles plugin discovery, loading, and lifecycle.
type Manager struct {
	pluginDir string
	stateDir  string
	client    service.ReorgClient
	llmClient llm.Client
	logger    hclog.Logger

	mu      sync.RWMutex
	plugins map[string]*LoadedPlugin
}

// LoadedPlugin represents a running plugin instance.
type LoadedPlugin struct {
	Name     string
	Path     string
	Manifest *plugin.Manifest
	Client   *goplugin.Client
	Plugin   plugin.Plugin
	Config   map[string]string
}

// ManagerConfig contains configuration for the plugin manager.
type ManagerConfig struct {
	PluginDir string
	StateDir  string
	Client    service.ReorgClient
	LLMClient llm.Client
	Logger    hclog.Logger
}

// NewManager creates a new plugin manager.
func NewManager(cfg ManagerConfig) *Manager {
	logger := cfg.Logger
	if logger == nil {
		logger = hclog.NewNullLogger()
	}

	return &Manager{
		pluginDir: cfg.PluginDir,
		stateDir:  cfg.StateDir,
		client:    cfg.Client,
		llmClient: cfg.LLMClient,
		logger:    logger,
		plugins:   make(map[string]*LoadedPlugin),
	}
}

// Discover finds all available plugins in the plugin directory.
func (m *Manager) Discover() ([]string, error) {
	if m.pluginDir == "" {
		return nil, fmt.Errorf("plugin directory not configured")
	}

	// Check if plugin directory exists
	if _, err := os.Stat(m.pluginDir); os.IsNotExist(err) {
		return []string{}, nil
	}

	var plugins []string
	entries, err := os.ReadDir(m.pluginDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read plugin directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()

		// Check for plugin naming convention: reorg-plugin-*
		if !strings.HasPrefix(name, "reorg-plugin-") {
			continue
		}

		// On Windows, check for .exe extension
		if runtime.GOOS == "windows" && !strings.HasSuffix(name, ".exe") {
			continue
		}

		// Check if executable
		path := filepath.Join(m.pluginDir, name)
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if info.Mode()&0111 == 0 && runtime.GOOS != "windows" {
			continue
		}

		// Extract plugin name from filename
		pluginName := strings.TrimPrefix(name, "reorg-plugin-")
		if runtime.GOOS == "windows" {
			pluginName = strings.TrimSuffix(pluginName, ".exe")
		}

		plugins = append(plugins, pluginName)
	}

	return plugins, nil
}

// Load loads a plugin by name and configures it.
func (m *Manager) Load(ctx context.Context, name string, config map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if already loaded
	if _, exists := m.plugins[name]; exists {
		return fmt.Errorf("plugin %s is already loaded", name)
	}

	// Find plugin path
	pluginPath := m.findPluginPath(name)
	if pluginPath == "" {
		return fmt.Errorf("plugin %s not found", name)
	}

	m.logger.Info("loading plugin", "name", name, "path", pluginPath)

	// Create plugin client
	client := goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig:  plugin.Handshake,
		Plugins:          plugin.PluginMap,
		Cmd:              exec.Command(pluginPath),
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
		Logger:           m.logger.Named(name),
	})

	// Connect to the plugin
	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return fmt.Errorf("failed to connect to plugin %s: %w", name, err)
	}

	// Request the plugin
	raw, err := rpcClient.Dispense(plugin.PluginName)
	if err != nil {
		client.Kill()
		return fmt.Errorf("failed to dispense plugin %s: %w", name, err)
	}

	p, ok := raw.(plugin.Plugin)
	if !ok {
		client.Kill()
		return fmt.Errorf("plugin %s does not implement Plugin interface", name)
	}

	// Get manifest
	manifest, err := p.GetManifest(ctx)
	if err != nil {
		client.Kill()
		return fmt.Errorf("failed to get manifest from plugin %s: %w", name, err)
	}

	// Create host client for plugin callbacks
	hostClient := NewHostClient(m.client, m.llmClient, filepath.Join(m.stateDir, "plugins", name))

	// Configure the plugin
	stateDir := filepath.Join(m.stateDir, "plugins", name)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		client.Kill()
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	if err := p.Configure(ctx, hostClient, config, stateDir); err != nil {
		client.Kill()
		return fmt.Errorf("failed to configure plugin %s: %w", name, err)
	}

	m.plugins[name] = &LoadedPlugin{
		Name:     name,
		Path:     pluginPath,
		Manifest: manifest,
		Client:   client,
		Plugin:   p,
		Config:   config,
	}

	m.logger.Info("plugin loaded successfully", "name", name, "version", manifest.Version)
	return nil
}

// Unload stops and removes a loaded plugin.
func (m *Manager) Unload(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	loaded, exists := m.plugins[name]
	if !exists {
		return fmt.Errorf("plugin %s is not loaded", name)
	}

	// Shutdown the plugin gracefully
	if err := loaded.Plugin.Shutdown(ctx); err != nil {
		m.logger.Warn("plugin shutdown returned error", "name", name, "error", err)
	}

	// Kill the plugin process
	loaded.Client.Kill()

	delete(m.plugins, name)
	m.logger.Info("plugin unloaded", "name", name)
	return nil
}

// Execute runs a loaded plugin.
func (m *Manager) Execute(ctx context.Context, name string, params *plugin.ExecuteParams) (*plugin.ExecuteResult, error) {
	m.mu.RLock()
	loaded, exists := m.plugins[name]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("plugin %s is not loaded", name)
	}

	m.logger.Info("executing plugin", "name", name, "dry_run", params.DryRun)

	result, err := loaded.Plugin.Execute(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("plugin execution failed: %w", err)
	}

	if result.Summary != nil {
		m.logger.Info("plugin execution complete",
			"name", name,
			"processed", result.Summary.ItemsProcessed,
			"imported", result.Summary.ItemsImported,
			"skipped", result.Summary.ItemsSkipped,
			"failed", result.Summary.ItemsFailed,
		)
	}

	return result, nil
}

// GetManifest returns the manifest for a plugin (loading it temporarily if needed).
func (m *Manager) GetManifest(ctx context.Context, name string) (*plugin.Manifest, error) {
	m.mu.RLock()
	loaded, exists := m.plugins[name]
	m.mu.RUnlock()

	if exists {
		return loaded.Manifest, nil
	}

	// Need to load the plugin temporarily to get its manifest
	pluginPath := m.findPluginPath(name)
	if pluginPath == "" {
		return nil, fmt.Errorf("plugin %s not found", name)
	}

	client := goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig:  plugin.Handshake,
		Plugins:          plugin.PluginMap,
		Cmd:              exec.Command(pluginPath),
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
		Logger:           m.logger.Named(name),
	})
	defer client.Kill()

	rpcClient, err := client.Client()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to plugin: %w", err)
	}

	raw, err := rpcClient.Dispense(plugin.PluginName)
	if err != nil {
		return nil, fmt.Errorf("failed to dispense plugin: %w", err)
	}

	p, ok := raw.(plugin.Plugin)
	if !ok {
		return nil, fmt.Errorf("plugin does not implement Plugin interface")
	}

	return p.GetManifest(ctx)
}

// ListLoaded returns the names of all loaded plugins.
func (m *Manager) ListLoaded() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var names []string
	for name := range m.plugins {
		names = append(names, name)
	}
	return names
}

// GetLoaded returns a loaded plugin by name.
func (m *Manager) GetLoaded(name string) (*LoadedPlugin, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	p, ok := m.plugins[name]
	return p, ok
}

// Shutdown stops all loaded plugins.
func (m *Manager) Shutdown(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, loaded := range m.plugins {
		m.logger.Info("shutting down plugin", "name", name)
		if err := loaded.Plugin.Shutdown(ctx); err != nil {
			m.logger.Warn("plugin shutdown error", "name", name, "error", err)
		}
		loaded.Client.Kill()
	}

	m.plugins = make(map[string]*LoadedPlugin)
}

// findPluginPath finds the full path to a plugin executable.
func (m *Manager) findPluginPath(name string) string {
	// Check plugin directory
	var filename string
	if runtime.GOOS == "windows" {
		filename = fmt.Sprintf("reorg-plugin-%s.exe", name)
	} else {
		filename = fmt.Sprintf("reorg-plugin-%s", name)
	}

	path := filepath.Join(m.pluginDir, filename)
	if _, err := os.Stat(path); err == nil {
		return path
	}

	// Check if the plugin is in PATH (for built-in plugins)
	if execPath, err := exec.LookPath(filename); err == nil {
		return execPath
	}

	return ""
}

// SetPluginDir sets the plugin directory.
func (m *Manager) SetPluginDir(dir string) {
	m.pluginDir = dir
}

// SetStateDir sets the state directory.
func (m *Manager) SetStateDir(dir string) {
	m.stateDir = dir
}
