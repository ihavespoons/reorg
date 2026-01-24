package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/hashicorp/go-hclog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/ihavespoons/reorg/internal/daemon"
	"github.com/ihavespoons/reorg/internal/service"
	"github.com/ihavespoons/reorg/internal/storage/markdown"
)

var (
	daemonOnceFlag   bool
	daemonPluginFlag string
	daemonDryRunFlag bool
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run the reorg daemon for scheduled plugin execution",
	Long: `Run the reorg daemon in the foreground.

The daemon loads plugins from the plugin directory and executes them
according to their configured schedules.

Examples:
  reorg daemon                       # Run continuously with scheduling
  reorg daemon --once                # Run all plugins once and exit
  reorg daemon --plugin apple-notes  # Run specific plugin only
  reorg daemon --once --dry-run      # Test run without making changes`,
	RunE: runDaemon,
}

func init() {
	rootCmd.AddCommand(daemonCmd)

	daemonCmd.Flags().BoolVar(&daemonOnceFlag, "once", false, "Run all plugins once and exit")
	daemonCmd.Flags().StringVar(&daemonPluginFlag, "plugin", "", "Run only the specified plugin")
	daemonCmd.Flags().BoolVar(&daemonDryRunFlag, "dry-run", false, "Perform a dry run without making changes")
}

func runDaemon(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Check if initialized
	if _, err := os.Stat(filepath.Join(dataDir, "areas")); os.IsNotExist(err) {
		return fmt.Errorf("reorg not initialized. Run 'reorg init' first")
	}

	// Setup logger
	logger := hclog.New(&hclog.LoggerOptions{
		Name:   "reorg-daemon",
		Level:  hclog.Info,
		Output: os.Stdout,
	})

	// Initialize store and client
	store := markdown.NewStore(dataDir)
	localClient := service.NewLocalClient(store)

	// Get LLM client (optional)
	llmClient, err := getLLMClient()
	if err != nil {
		logger.Warn("LLM client not available", "error", err)
	}

	// Get plugin configuration
	pluginDir := viper.GetString("plugins.dir")
	if pluginDir == "" {
		pluginDir = filepath.Join(dataDir, "plugins")
	}
	// Expand ~ in path
	if len(pluginDir) >= 2 && pluginDir[:2] == "~/" {
		home, _ := os.UserHomeDir()
		pluginDir = filepath.Join(home, pluginDir[2:])
	}

	stateDir := filepath.Join(dataDir, "state")

	// Load plugin configurations from config
	pluginConfigs := loadPluginConfigs()

	// Create daemon
	d := daemon.NewDaemon(daemon.Config{
		PluginDir: pluginDir,
		StateDir:  stateDir,
		Client:    localClient,
		LLMClient: llmClient,
		Logger:    logger,
		Plugins:   pluginConfigs,
	})

	// Handle specific plugin mode
	if daemonPluginFlag != "" {
		return runSinglePlugin(ctx, d, daemonPluginFlag, pluginConfigs, logger)
	}

	// Handle run-once mode
	if daemonOnceFlag {
		fmt.Println(titleStyle.Render("\n  Reorg Daemon - One-time Run\n"))
		return d.RunOnce(ctx, pluginConfigs)
	}

	// Run continuously with scheduling
	fmt.Println(titleStyle.Render("\n  Reorg Daemon\n"))
	fmt.Printf("Plugin directory: %s\n", pluginDir)
	fmt.Printf("State directory: %s\n", stateDir)
	fmt.Println()

	if err := d.Start(ctx, pluginConfigs); err != nil {
		return fmt.Errorf("failed to start daemon: %w", err)
	}

	// Show scheduled plugins
	scheduled := d.ListScheduled()
	if len(scheduled) > 0 {
		fmt.Println("Scheduled plugins:")
		for _, sp := range scheduled {
			fmt.Printf("  %s: next run at %s\n", sp.Name, sp.NextRun.Format("15:04:05"))
		}
		fmt.Println()
	}

	fmt.Println("Daemon running. Press Ctrl+C to stop.")

	// Wait for signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	<-sigCh
	fmt.Println("\nShutting down...")
	d.Stop(ctx)

	return nil
}

func runSinglePlugin(ctx context.Context, d *daemon.Daemon, pluginName string, configs map[string]daemon.PluginConfig, logger hclog.Logger) error {
	fmt.Println(titleStyle.Render(fmt.Sprintf("\n  Running Plugin: %s\n", pluginName)))

	if daemonDryRunFlag {
		fmt.Println(dimStyle.Render("(Dry run mode - no changes will be made)"))
		fmt.Println()
	}

	// Get plugin config
	cfg, exists := configs[pluginName]
	if !exists {
		cfg = daemon.PluginConfig{Enabled: true}
	}

	result, err := d.RunPluginOnce(ctx, pluginName, cfg, daemonDryRunFlag)
	if err != nil {
		return fmt.Errorf("plugin execution failed: %w", err)
	}

	// Show results
	if result.Summary != nil {
		fmt.Printf("Processed: %d\n", result.Summary.ItemsProcessed)
		fmt.Printf("Imported:  %d\n", result.Summary.ItemsImported)
		fmt.Printf("Skipped:   %d\n", result.Summary.ItemsSkipped)
		fmt.Printf("Failed:    %d\n", result.Summary.ItemsFailed)
		if result.Summary.Message != "" {
			fmt.Printf("Message:   %s\n", result.Summary.Message)
		}
	}

	if !result.Success && result.Error != "" {
		return fmt.Errorf("plugin error: %s", result.Error)
	}

	return nil
}

func loadPluginConfigs() map[string]daemon.PluginConfig {
	configs := make(map[string]daemon.PluginConfig)

	// Get plugin configs from viper
	pluginsConfig := viper.GetStringMap("plugins")
	for key, value := range pluginsConfig {
		if key == "dir" {
			continue
		}

		cfg := daemon.PluginConfig{Enabled: true}

		if m, ok := value.(map[string]interface{}); ok {
			if enabled, ok := m["enabled"].(bool); ok {
				cfg.Enabled = enabled
			}
			if schedule, ok := m["schedule"].(string); ok {
				cfg.Schedule = schedule
			}
			if configMap, ok := m["config"].(map[string]interface{}); ok {
				cfg.Config = make(map[string]string)
				for k, v := range configMap {
					if s, ok := v.(string); ok {
						cfg.Config[k] = s
					}
				}
			}
		}

		configs[key] = cfg
	}

	return configs
}
