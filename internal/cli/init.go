package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/spf13/cobra"

	"github.com/ihavespoons/reorg/internal/domain"
	"github.com/ihavespoons/reorg/internal/storage/markdown"
)

var (
	initSkipWizard bool
	initWithGit    bool
)

// Styles
var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	promptStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new reorg data directory",
	Long: `Initialize a new reorg data directory with the required structure.

This command will:
1. Create the data directory structure
2. Optionally initialize git for version control
3. Interactively create default areas (Work, Personal, Life Admin)

You can skip the interactive wizard with --skip-wizard.`,
	RunE: runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)

	initCmd.Flags().BoolVar(&initSkipWizard, "skip-wizard", false, "Skip interactive area creation wizard")
	initCmd.Flags().BoolVar(&initWithGit, "git", true, "Initialize git repository")
}

func runInit(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	fmt.Println(titleStyle.Render("\n  Reorg - Personal Organization Tool\n"))

	// Check if already initialized
	if _, err := os.Stat(filepath.Join(dataDir, "areas")); err == nil {
		return fmt.Errorf("reorg is already initialized at %s", dataDir)
	}

	fmt.Printf("Initializing reorg in %s\n\n", dimStyle.Render(dataDir))

	// Create store and initialize directory structure
	store := markdown.NewStore(dataDir)
	if err := store.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize: %w", err)
	}

	fmt.Println(successStyle.Render("✓") + " Created directory structure")

	// Initialize git if requested
	if initWithGit {
		if err := initGit(dataDir); err != nil {
			fmt.Printf("  Warning: failed to initialize git: %v\n", err)
		} else {
			fmt.Println(successStyle.Render("✓") + " Initialized git repository")
		}
	}

	// Create config file
	if err := createDefaultConfig(dataDir); err != nil {
		fmt.Printf("  Warning: failed to create config: %v\n", err)
	} else {
		fmt.Println(successStyle.Render("✓") + " Created config.yaml")
	}

	// Interactive area creation
	if !initSkipWizard {
		fmt.Println()
		if err := runAreaWizard(ctx, store); err != nil {
			return err
		}
	}

	fmt.Println()
	fmt.Println(successStyle.Render("Reorg initialized successfully!"))
	fmt.Println()
	fmt.Println("Get started:")
	fmt.Println(dimStyle.Render("  reorg area list        # List your areas"))
	fmt.Println(dimStyle.Render("  reorg project create   # Create a new project"))
	fmt.Println(dimStyle.Render("  reorg task create      # Create a new task"))
	fmt.Println()

	return nil
}

func runAreaWizard(ctx context.Context, store *markdown.Store) error {
	reader := bufio.NewReader(os.Stdin)
	defaultAreas := domain.DefaultAreas()

	fmt.Println(promptStyle.Render("Which areas would you like to create?"))
	fmt.Println(dimStyle.Render("(Areas are top-level categories for organizing projects)\n"))

	for _, area := range defaultAreas {
		fmt.Printf("  Create %s area? [Y/n]: ", titleStyle.Render(area.Title))

		input, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}

		input = strings.TrimSpace(strings.ToLower(input))
		if input == "" || input == "y" || input == "yes" {
			if err := store.Areas().Create(ctx, area); err != nil {
				return fmt.Errorf("failed to create area %s: %w", area.Title, err)
			}
			fmt.Println(successStyle.Render("  ✓") + " Created " + area.Title)
		} else {
			fmt.Println(dimStyle.Render("  ○ Skipped " + area.Title))
		}
	}

	// Offer to create custom area
	fmt.Println()
	fmt.Print("  Create a custom area? [y/N]: ")
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))

	if input == "y" || input == "yes" {
		for {
			fmt.Print("  Area name (or empty to finish): ")
			name, _ := reader.ReadString('\n')
			name = strings.TrimSpace(name)

			if name == "" {
				break
			}

			area := domain.NewArea(name)
			if err := store.Areas().Create(ctx, area); err != nil {
				fmt.Printf("  Error: %v\n", err)
				continue
			}
			fmt.Println(successStyle.Render("  ✓") + " Created " + name)
		}
	}

	return nil
}

func initGit(dir string) error {
	gitDir := filepath.Join(dir, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		// Already a git repo
		return nil
	}

	// Initialize git repository using go-git with "main" as default branch
	_, err := git.PlainInitWithOptions(dir, &git.PlainInitOptions{
		InitOptions: git.InitOptions{
			DefaultBranch: plumbing.NewBranchReferenceName("main"),
		},
		Bare: false,
	})
	if err != nil {
		return fmt.Errorf("failed to init git: %w", err)
	}

	// Create .gitignore
	gitignore := `# Reorg gitignore
*.swp
*.swo
*~
.DS_Store
`
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(gitignore), 0644); err != nil {
		return err
	}

	return nil
}

func createDefaultConfig(dir string) error {
	configPath := filepath.Join(dir, "config.yaml")

	config := `# Reorg Configuration

# Data directory (this file's location)
# data_dir: ~/.reorg

# Server mode: embedded or remote
mode: embedded

# Git settings
git:
  enabled: true
  auto_commit: true
  commit_message_prefix: "reorg: "

# LLM settings
# llm:
#   provider: claude
#   api_key: ${ANTHROPIC_API_KEY}

# Plugin settings
plugins:
  # Directory containing plugin executables
  dir: ~/.reorg/plugins

  # Apple Notes importer plugin
  apple-notes-importer:
    enabled: true
    # schedule: "*/15 * * * *"  # Override default schedule (every 15 min)
    config:
      since: "24h"              # Import notes from last 24 hours
      auto: "true"              # Auto-accept AI categorizations
      # folders: ""             # Comma-separated folders to import

  # Obsidian importer plugin
  obsidian-importer:
    enabled: false
    # schedule: "0 */6 * * *"   # Override default schedule (every 6 hours)
    config:
      vault_path: ""            # Path to Obsidian vault (required)
      since: "24h"              # Import notes from last 24 hours
      # folders: ""             # Comma-separated folders to import
      # skip_dirs: ".obsidian,.git,.trash"

# CLI settings
cli:
  color: true
  date_format: "2006-01-02"

# Default values
defaults:
  priority: medium
  task_status: pending
`

	return os.WriteFile(configPath, []byte(config), 0644)
}
