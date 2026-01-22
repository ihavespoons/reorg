package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	apiclient "github.com/beng/reorg/internal/api/client"
	"github.com/beng/reorg/internal/service"
	"github.com/beng/reorg/internal/storage/markdown"
)

var (
	cfgFile       string
	dataDir       string
	mode          string
	serverAddress string
	store         *markdown.Store
	client        service.ReorgClient

	// Version info set by main
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// SetVersion sets the version information
func SetVersion(v, c, d string) {
	version = v
	commit = c
	date = d
}

// rootCmd represents the base command
var rootCmd = &cobra.Command{
	Use:   "reorg",
	Short: "A personal organization tool",
	Long: `Reorg is a personal organization tool that helps you manage
areas, projects, and tasks using markdown files stored in git.

It supports a hierarchical structure:
  Areas (Work, Personal, Life Admin) > Projects > Tasks

All data is stored as markdown files with YAML frontmatter,
making it easy to edit manually and track with version control.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip client initialization for commands that don't need it
		switch cmd.Name() {
		case "init", "serve", "version", "help", "completion":
			return nil
		}

		// Initialize client based on mode
		return initClient()
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ~/.reorg/config.yaml)")
	rootCmd.PersistentFlags().StringVar(&dataDir, "data-dir", "", "data directory (default is ~/.reorg)")
	rootCmd.PersistentFlags().StringVar(&mode, "mode", "", "operation mode: embedded or remote (default is embedded)")
	rootCmd.PersistentFlags().StringVar(&serverAddress, "server", "", "server address for remote mode (default is localhost:50051)")

	// Bind flags to viper
	viper.BindPFlag("data_dir", rootCmd.PersistentFlags().Lookup("data-dir"))
	viper.BindPFlag("mode", rootCmd.PersistentFlags().Lookup("mode"))
	viper.BindPFlag("server.address", rootCmd.PersistentFlags().Lookup("server"))
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error finding home directory:", err)
			os.Exit(1)
		}

		// Default config location
		configDir := filepath.Join(home, ".reorg")
		viper.AddConfigPath(configDir)
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
	}

	// Read in environment variables that match
	viper.SetEnvPrefix("REORG")
	viper.AutomaticEnv()

	// Read config file if it exists
	if err := viper.ReadInConfig(); err == nil {
		// Config loaded successfully
	}

	// Set data directory
	if dataDir == "" {
		dataDir = viper.GetString("data_dir")
	}
	if dataDir == "" {
		home, _ := os.UserHomeDir()
		dataDir = filepath.Join(home, ".reorg")
	}

	// Expand ~ in path
	if len(dataDir) >= 2 && dataDir[:2] == "~/" {
		home, _ := os.UserHomeDir()
		dataDir = filepath.Join(home, dataDir[2:])
	}

	// Set mode
	if mode == "" {
		mode = viper.GetString("mode")
	}
	if mode == "" {
		mode = "embedded"
	}

	// Set server address
	if serverAddress == "" {
		serverAddress = viper.GetString("server.address")
	}
	if serverAddress == "" {
		serverAddress = "localhost:50051"
	}
}

// initClient initializes the appropriate client based on mode
func initClient() error {
	switch mode {
	case "remote":
		// Connect to remote server
		remoteClient, err := apiclient.NewRemoteClient(serverAddress)
		if err != nil {
			return fmt.Errorf("failed to connect to server: %w", err)
		}
		client = remoteClient
		return nil

	case "embedded":
		fallthrough
	default:
		// Check if initialized
		if _, err := os.Stat(filepath.Join(dataDir, "areas")); os.IsNotExist(err) {
			return fmt.Errorf("reorg not initialized. Run 'reorg init' first")
		}

		// Initialize local store and client
		store = markdown.NewStore(dataDir)
		client = service.NewLocalClient(store)
		return nil
	}
}

// GetClient returns the initialized client
func GetClient() service.ReorgClient {
	return client
}

// GetStore returns the initialized store (for embedded mode)
func GetStore() *markdown.Store {
	return store
}

// GetDataDir returns the data directory path
func GetDataDir() string {
	return dataDir
}

// GetMode returns the current operation mode
func GetMode() string {
	return mode
}
