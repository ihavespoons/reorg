package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/hashicorp/go-hclog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/ihavespoons/reorg/internal/daemon"
	"github.com/ihavespoons/reorg/internal/service"
	"github.com/ihavespoons/reorg/internal/storage/markdown"
)

var (
	pluginDryRunFlag bool
)

var pluginCmd = &cobra.Command{
	Use:   "plugin",
	Short: "Manage reorg plugins",
	Long: `Manage reorg plugins for importing and syncing data from external sources.

Plugins are separate executables that extend reorg's functionality.
They can import data from Apple Notes, Obsidian, and other sources.`,
}

var pluginListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available plugins",
	Long:  `List all available plugins in the plugin directory.`,
	RunE:  runPluginList,
}

var pluginRunCmd = &cobra.Command{
	Use:   "run <plugin-name>",
	Short: "Run a plugin manually",
	Long: `Run a specific plugin manually.

Example:
  reorg plugin run apple-notes
  reorg plugin run apple-notes --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: runPluginRun,
}

var pluginInfoCmd = &cobra.Command{
	Use:   "info <plugin-name>",
	Short: "Show plugin information",
	Long:  `Display detailed information about a plugin including its manifest.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runPluginInfo,
}

func init() {
	rootCmd.AddCommand(pluginCmd)
	pluginCmd.AddCommand(pluginListCmd)
	pluginCmd.AddCommand(pluginRunCmd)
	pluginCmd.AddCommand(pluginInfoCmd)

	pluginRunCmd.Flags().BoolVar(&pluginDryRunFlag, "dry-run", false, "Perform a dry run without making changes")
}

func getPluginDir() string {
	pluginDir := viper.GetString("plugins.dir")
	if pluginDir == "" {
		pluginDir = filepath.Join(dataDir, "plugins")
	}
	// Expand ~ in path
	if len(pluginDir) >= 2 && pluginDir[:2] == "~/" {
		home, _ := os.UserHomeDir()
		pluginDir = filepath.Join(home, pluginDir[2:])
	}
	return pluginDir
}

func createDaemon() (*daemon.Daemon, error) {
	// Initialize store and client
	store := markdown.NewStore(dataDir)
	localClient := service.NewLocalClient(store)

	// Get LLM client (optional)
	llmClient, _ := getLLMClient()

	pluginDir := getPluginDir()
	stateDir := filepath.Join(dataDir, "state")

	// Create silent logger for CLI usage
	logger := hclog.NewNullLogger()

	return daemon.NewDaemon(daemon.Config{
		PluginDir: pluginDir,
		StateDir:  stateDir,
		Client:    localClient,
		LLMClient: llmClient,
		Logger:    logger,
	}), nil
}

func runPluginList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	d, err := createDaemon()
	if err != nil {
		return err
	}

	fmt.Println(titleStyle.Render("\n  Available Plugins\n"))

	// List available plugins
	available, err := d.ListAvailable()
	if err != nil {
		return fmt.Errorf("failed to list plugins: %w", err)
	}

	if len(available) == 0 {
		fmt.Println(dimStyle.Render("No plugins found in " + getPluginDir()))
		fmt.Println()
		fmt.Println("To install plugins, place plugin executables in the plugin directory.")
		fmt.Println("Plugin executables should be named: reorg-plugin-<name>")
		return nil
	}

	// Load plugin configs
	pluginConfigs := loadPluginConfigs()

	// Style definitions
	headerStyle := lipgloss.NewStyle().Bold(true)
	enabledStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	disabledStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	for _, name := range available {
		cfg, exists := pluginConfigs[name]
		enabled := !exists || cfg.Enabled

		// Get manifest
		manifest, err := d.GetPluginInfo(ctx, name)
		if err != nil {
			fmt.Printf("%s - %s\n", headerStyle.Render(name), dimStyle.Render("(error loading: "+err.Error()+")"))
			continue
		}

		statusStr := enabledStyle.Render("enabled")
		if !enabled {
			statusStr = disabledStyle.Render("disabled")
		}

		fmt.Printf("%s v%s [%s]\n", headerStyle.Render(manifest.Name), manifest.Version, statusStr)
		if manifest.Description != "" {
			fmt.Printf("  %s\n", dimStyle.Render(manifest.Description))
		}
		if manifest.Schedule != "" {
			fmt.Printf("  Schedule: %s\n", manifest.Schedule)
		}
		if len(manifest.Capabilities) > 0 {
			fmt.Printf("  Capabilities: %s\n", strings.Join(manifest.Capabilities, ", "))
		}
		fmt.Println()
	}

	return nil
}

func runPluginRun(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	pluginName := args[0]

	d, err := createDaemon()
	if err != nil {
		return err
	}

	fmt.Println(titleStyle.Render(fmt.Sprintf("\n  Running Plugin: %s\n", pluginName)))

	if pluginDryRunFlag {
		fmt.Println(dimStyle.Render("(Dry run mode - no changes will be made)"))
		fmt.Println()
	}

	// Get plugin config
	pluginConfigs := loadPluginConfigs()
	cfg, exists := pluginConfigs[pluginName]
	if !exists {
		cfg = daemon.PluginConfig{Enabled: true}
	}

	result, err := d.RunPluginOnce(ctx, pluginName, cfg, pluginDryRunFlag)
	if err != nil {
		return fmt.Errorf("plugin execution failed: %w", err)
	}

	// Show results
	fmt.Println()
	if result.Summary != nil {
		fmt.Printf("Processed: %d\n", result.Summary.ItemsProcessed)
		fmt.Printf("Imported:  %d\n", result.Summary.ItemsImported)
		fmt.Printf("Skipped:   %d\n", result.Summary.ItemsSkipped)
		fmt.Printf("Failed:    %d\n", result.Summary.ItemsFailed)
		if result.Summary.Message != "" {
			fmt.Printf("\nMessage: %s\n", result.Summary.Message)
		}
	}

	// Show individual results if not too many
	if len(result.Results) > 0 && len(result.Results) <= 20 {
		fmt.Println()
		fmt.Println("Details:")
		for _, item := range result.Results {
			actionStyle := dimStyle
			switch item.Action {
			case "imported":
				actionStyle = successStyle
			case "failed":
				actionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
			}
			fmt.Printf("  %s %s", actionStyle.Render(fmt.Sprintf("[%s]", item.Action)), item.Name)
			if item.Message != "" {
				fmt.Printf(" - %s", dimStyle.Render(item.Message))
			}
			fmt.Println()
		}
	}

	if !result.Success && result.Error != "" {
		fmt.Println()
		return fmt.Errorf("plugin error: %s", result.Error)
	}

	fmt.Println()
	fmt.Println(successStyle.Render("Done."))
	return nil
}

func runPluginInfo(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	pluginName := args[0]

	d, err := createDaemon()
	if err != nil {
		return err
	}

	manifest, err := d.GetPluginInfo(ctx, pluginName)
	if err != nil {
		return fmt.Errorf("failed to get plugin info: %w", err)
	}

	fmt.Println(titleStyle.Render(fmt.Sprintf("\n  Plugin: %s\n", manifest.Name)))

	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Width(14)

	fmt.Printf("%s %s\n", labelStyle.Render("Version:"), manifest.Version)
	if manifest.Description != "" {
		fmt.Printf("%s %s\n", labelStyle.Render("Description:"), manifest.Description)
	}
	if manifest.Author != "" {
		fmt.Printf("%s %s\n", labelStyle.Render("Author:"), manifest.Author)
	}
	if manifest.Schedule != "" {
		fmt.Printf("%s %s\n", labelStyle.Render("Schedule:"), manifest.Schedule)
	}
	if len(manifest.Capabilities) > 0 {
		fmt.Printf("%s %s\n", labelStyle.Render("Capabilities:"), strings.Join(manifest.Capabilities, ", "))
	}

	// Show configuration from config file
	pluginConfigs := loadPluginConfigs()
	if cfg, exists := pluginConfigs[pluginName]; exists {
		fmt.Println()
		fmt.Println("Configuration:")
		if cfg.Enabled {
			fmt.Printf("  Enabled: %s\n", successStyle.Render("yes"))
		} else {
			fmt.Printf("  Enabled: %s\n", dimStyle.Render("no"))
		}
		if cfg.Schedule != "" {
			fmt.Printf("  Schedule override: %s\n", cfg.Schedule)
		}
		if len(cfg.Config) > 0 {
			fmt.Println("  Settings:")
			for k, v := range cfg.Config {
				fmt.Printf("    %s: %s\n", k, v)
			}
		}
	}

	fmt.Println()
	return nil
}
