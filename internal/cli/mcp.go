package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	mcpserver "github.com/ihavespoons/reorg/internal/mcp"
	"github.com/ihavespoons/reorg/internal/service"
	"github.com/ihavespoons/reorg/internal/storage/markdown"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start MCP server for Claude Desktop integration",
	Long: `Start the Model Context Protocol (MCP) server for integration with Claude Desktop.

This runs an MCP server over stdio that exposes reorg functionality as tools:
  - list_areas, create_area
  - list_projects, create_project, complete_project
  - list_tasks, create_task, complete_task, start_task
  - get_status

To use with Claude Desktop, add this to your claude_desktop_config.json:

  {
    "mcpServers": {
      "reorg": {
        "command": "reorg",
        "args": ["mcp"]
      }
    }
  }

Config file location:
  macOS: ~/Library/Application Support/Claude/claude_desktop_config.json
  Windows: %APPDATA%\Claude\claude_desktop_config.json`,
	RunE: runMCP,
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}

func runMCP(cmd *cobra.Command, args []string) error {
	// Initialize data directory
	if dataDir == "" {
		home, _ := os.UserHomeDir()
		dataDir = filepath.Join(home, ".reorg")
	}

	// Check if initialized
	if _, err := os.Stat(filepath.Join(dataDir, "areas")); os.IsNotExist(err) {
		return fmt.Errorf("reorg not initialized. Run 'reorg init' first")
	}

	// Initialize local store and client
	store := markdown.NewStore(dataDir)
	client := service.NewLocalClient(store)

	// Create and run MCP server
	server := mcpserver.NewServer(client)
	return server.Run(context.Background())
}
