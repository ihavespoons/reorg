// Package daemon provides the background daemon for scheduled plugin execution.
package daemon

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/robfig/cron/v3"

	"github.com/ihavespoons/reorg/internal/llm"
	intplugin "github.com/ihavespoons/reorg/internal/plugin"
	"github.com/ihavespoons/reorg/internal/service"
	"github.com/ihavespoons/reorg/pkg/plugin"
)

// Daemon manages scheduled plugin execution.
type Daemon struct {
	manager   *intplugin.Manager
	scheduler *cron.Cron
	logger    hclog.Logger

	mu   sync.RWMutex
	jobs map[string]cron.EntryID
}

// Config contains daemon configuration.
type Config struct {
	PluginDir string
	StateDir  string
	Client    service.ReorgClient
	LLMClient llm.Client
	Logger    hclog.Logger

	// Plugin configurations from config file
	Plugins map[string]PluginConfig
}

// PluginConfig contains per-plugin configuration.
type PluginConfig struct {
	Enabled  bool              `yaml:"enabled" mapstructure:"enabled"`
	Schedule string            `yaml:"schedule" mapstructure:"schedule"` // Override default schedule
	Config   map[string]string `yaml:"config" mapstructure:"config"`
}

// ScheduledPlugin represents a plugin scheduled for execution.
type ScheduledPlugin struct {
	Name     string
	Schedule string
	NextRun  time.Time
	LastRun  time.Time
	Enabled  bool
}

// NewDaemon creates a new daemon.
func NewDaemon(cfg Config) *Daemon {
	logger := cfg.Logger
	if logger == nil {
		logger = hclog.NewNullLogger()
	}

	manager := intplugin.NewManager(intplugin.ManagerConfig{
		PluginDir: cfg.PluginDir,
		StateDir:  cfg.StateDir,
		Client:    cfg.Client,
		LLMClient: cfg.LLMClient,
		Logger:    logger.Named("plugin-manager"),
	})

	return &Daemon{
		manager:   manager,
		scheduler: cron.New(cron.WithSeconds()),
		logger:    logger,
		jobs:      make(map[string]cron.EntryID),
	}
}

// Start starts the daemon and schedules all enabled plugins.
func (d *Daemon) Start(ctx context.Context, plugins map[string]PluginConfig) error {
	d.logger.Info("starting daemon")

	// Discover available plugins
	available, err := d.manager.Discover()
	if err != nil {
		d.logger.Warn("failed to discover plugins", "error", err)
	}

	d.logger.Info("discovered plugins", "count", len(available), "plugins", available)

	// Schedule each enabled plugin
	for _, name := range available {
		pluginCfg, exists := plugins[name]
		if !exists {
			// Use default config if not specified
			pluginCfg = PluginConfig{Enabled: true}
		}

		if !pluginCfg.Enabled {
			d.logger.Info("plugin disabled", "name", name)
			continue
		}

		if err := d.schedulePlugin(ctx, name, pluginCfg); err != nil {
			d.logger.Error("failed to schedule plugin", "name", name, "error", err)
			continue
		}
	}

	// Start the scheduler
	d.scheduler.Start()
	d.logger.Info("daemon started", "scheduled_plugins", len(d.jobs))

	return nil
}

// schedulePlugin loads and schedules a single plugin.
func (d *Daemon) schedulePlugin(ctx context.Context, name string, cfg PluginConfig) error {
	// Get plugin manifest to determine schedule
	manifest, err := d.manager.GetManifest(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to get manifest: %w", err)
	}

	// Use configured schedule or default from manifest
	schedule := cfg.Schedule
	if schedule == "" {
		schedule = manifest.Schedule
	}
	if schedule == "" {
		d.logger.Info("plugin has no schedule, skipping", "name", name)
		return nil
	}

	// Load the plugin
	if err := d.manager.Load(ctx, name, cfg.Config); err != nil {
		return fmt.Errorf("failed to load plugin: %w", err)
	}

	// Schedule the plugin execution
	entryID, err := d.scheduler.AddFunc(schedule, func() {
		d.executePlugin(name)
	})
	if err != nil {
		return fmt.Errorf("failed to schedule: %w", err)
	}

	d.mu.Lock()
	d.jobs[name] = entryID
	d.mu.Unlock()

	d.logger.Info("scheduled plugin", "name", name, "schedule", schedule)
	return nil
}

// executePlugin runs a single plugin.
func (d *Daemon) executePlugin(name string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	d.logger.Info("executing scheduled plugin", "name", name)

	result, err := d.manager.Execute(ctx, name, &plugin.ExecuteParams{
		DryRun: false,
	})
	if err != nil {
		d.logger.Error("plugin execution failed", "name", name, "error", err)
		return
	}

	if result.Summary != nil {
		d.logger.Info("plugin execution complete",
			"name", name,
			"processed", result.Summary.ItemsProcessed,
			"imported", result.Summary.ItemsImported,
			"skipped", result.Summary.ItemsSkipped,
			"failed", result.Summary.ItemsFailed,
		)
	}
}

// Stop stops the daemon and all scheduled plugins.
func (d *Daemon) Stop(ctx context.Context) {
	d.logger.Info("stopping daemon")

	// Stop the scheduler
	d.scheduler.Stop()

	// Shutdown all loaded plugins
	d.manager.Shutdown(ctx)

	d.mu.Lock()
	d.jobs = make(map[string]cron.EntryID)
	d.mu.Unlock()

	d.logger.Info("daemon stopped")
}

// TriggerPlugin manually triggers a plugin execution.
func (d *Daemon) TriggerPlugin(ctx context.Context, name string, dryRun bool) (*plugin.ExecuteResult, error) {
	d.logger.Info("manually triggering plugin", "name", name, "dry_run", dryRun)

	// Check if plugin is loaded
	if _, exists := d.manager.GetLoaded(name); !exists {
		// Try to load it
		if err := d.manager.Load(ctx, name, nil); err != nil {
			return nil, fmt.Errorf("failed to load plugin: %w", err)
		}
	}

	return d.manager.Execute(ctx, name, &plugin.ExecuteParams{
		DryRun: dryRun,
	})
}

// RunOnce runs all enabled plugins once and exits.
func (d *Daemon) RunOnce(ctx context.Context, plugins map[string]PluginConfig) error {
	d.logger.Info("running all plugins once")

	// Discover available plugins
	available, err := d.manager.Discover()
	if err != nil {
		d.logger.Warn("failed to discover plugins", "error", err)
	}

	for _, name := range available {
		pluginCfg, exists := plugins[name]
		if !exists {
			pluginCfg = PluginConfig{Enabled: true}
		}

		if !pluginCfg.Enabled {
			d.logger.Info("plugin disabled, skipping", "name", name)
			continue
		}

		// Load and execute
		if err := d.manager.Load(ctx, name, pluginCfg.Config); err != nil {
			d.logger.Error("failed to load plugin", "name", name, "error", err)
			continue
		}

		result, err := d.manager.Execute(ctx, name, &plugin.ExecuteParams{
			DryRun: false,
		})
		if err != nil {
			d.logger.Error("plugin execution failed", "name", name, "error", err)
			continue
		}

		if result.Summary != nil {
			d.logger.Info("plugin execution complete",
				"name", name,
				"processed", result.Summary.ItemsProcessed,
				"imported", result.Summary.ItemsImported,
				"skipped", result.Summary.ItemsSkipped,
				"failed", result.Summary.ItemsFailed,
			)
		}

		// Unload after execution in run-once mode
		if err := d.manager.Unload(ctx, name); err != nil {
			d.logger.Warn("failed to unload plugin", "name", name, "error", err)
		}
	}

	return nil
}

// RunPluginOnce runs a specific plugin once.
func (d *Daemon) RunPluginOnce(ctx context.Context, name string, cfg PluginConfig, dryRun bool) (*plugin.ExecuteResult, error) {
	d.logger.Info("running plugin once", "name", name, "dry_run", dryRun)

	// Load the plugin
	if err := d.manager.Load(ctx, name, cfg.Config); err != nil {
		return nil, fmt.Errorf("failed to load plugin: %w", err)
	}
	defer func() {
		if err := d.manager.Unload(ctx, name); err != nil {
			d.logger.Warn("failed to unload plugin", "name", name, "error", err)
		}
	}()

	return d.manager.Execute(ctx, name, &plugin.ExecuteParams{
		DryRun: dryRun,
	})
}

// ListScheduled returns information about scheduled plugins.
func (d *Daemon) ListScheduled() []ScheduledPlugin {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var result []ScheduledPlugin
	for name, entryID := range d.jobs {
		entry := d.scheduler.Entry(entryID)
		loaded, _ := d.manager.GetLoaded(name)

		sp := ScheduledPlugin{
			Name:    name,
			NextRun: entry.Next,
			Enabled: true,
		}

		if loaded != nil && loaded.Manifest != nil {
			sp.Schedule = loaded.Manifest.Schedule
		}

		result = append(result, sp)
	}

	return result
}

// ListAvailable returns names of all available plugins.
func (d *Daemon) ListAvailable() ([]string, error) {
	return d.manager.Discover()
}

// GetPluginInfo returns detailed information about a plugin.
func (d *Daemon) GetPluginInfo(ctx context.Context, name string) (*plugin.Manifest, error) {
	return d.manager.GetManifest(ctx, name)
}

// GetManager returns the plugin manager for direct access.
func (d *Daemon) GetManager() *intplugin.Manager {
	return d.manager
}
