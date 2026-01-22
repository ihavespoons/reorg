package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/beng/reorg/internal/domain"
	"github.com/beng/reorg/internal/integrations/apple_notes"
	"github.com/beng/reorg/internal/integrations/obsidian"
	"github.com/beng/reorg/internal/llm"
)

var (
	importSinceFlag    string
	importFolderFlag   string
	importDryRunFlag   bool
	importAutoFlag     bool
	importVaultFlag    string
)

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "Import notes from external sources",
	Long:  `Import notes from Apple Notes, Obsidian vaults, or markdown folders.`,
}

var importNotesCmd = &cobra.Command{
	Use:   "notes",
	Short: "Import from Apple Notes",
	Long: `Import notes from Apple Notes and categorize them using AI.

The import process:
1. Reads notes from Apple Notes via AppleScript
2. Uses AI to categorize each note (work/personal/life-admin)
3. Extracts actionable tasks from notes
4. Creates projects and tasks in reorg`,
	RunE: runImportNotes,
}

var importObsidianCmd = &cobra.Command{
	Use:   "obsidian [vault-path]",
	Short: "Import from Obsidian vault",
	Long: `Import notes from an Obsidian vault or markdown folder.

The import process:
1. Reads markdown files from the specified directory
2. Uses AI to categorize each note (work/personal/life-admin)
3. Extracts actionable tasks from notes
4. Creates projects and tasks in reorg`,
	Args: cobra.MaximumNArgs(1),
	RunE: runImportObsidian,
}

var importInboxCmd = &cobra.Command{
	Use:   "inbox",
	Short: "Process inbox items",
	Long: `Process items in the reorg inbox folder using AI.

Items in ~/.reorg/inbox/ will be:
1. Categorized into the appropriate area
2. Converted to projects or tasks
3. Moved to their proper location`,
	RunE: runImportInbox,
}

func init() {
	rootCmd.AddCommand(importCmd)
	importCmd.AddCommand(importNotesCmd)
	importCmd.AddCommand(importObsidianCmd)
	importCmd.AddCommand(importInboxCmd)

	// Apple Notes flags
	importNotesCmd.Flags().StringVar(&importSinceFlag, "since", "24h", "Import notes modified within this duration (e.g., 24h, 7d)")
	importNotesCmd.Flags().StringVar(&importFolderFlag, "folder", "", "Only import from this folder")
	importNotesCmd.Flags().BoolVar(&importDryRunFlag, "dry-run", false, "Show what would be imported without making changes")
	importNotesCmd.Flags().BoolVar(&importAutoFlag, "auto", false, "Automatically accept AI categorizations")

	// Obsidian flags
	importObsidianCmd.Flags().StringVar(&importSinceFlag, "since", "", "Import notes modified within this duration")
	importObsidianCmd.Flags().StringVar(&importFolderFlag, "folder", "", "Only import from this subfolder")
	importObsidianCmd.Flags().BoolVar(&importDryRunFlag, "dry-run", false, "Show what would be imported")
	importObsidianCmd.Flags().BoolVar(&importAutoFlag, "auto", false, "Auto-accept categorizations")
	importObsidianCmd.Flags().StringVar(&importVaultFlag, "vault", "", "Obsidian vault path (can also be set in config)")
}

func getLLMClient() (llm.Client, error) {
	provider := viper.GetString("llm.provider")
	model := viper.GetString("llm.model")
	baseURL := viper.GetString("llm.base_url")

	// Get credentials from config (Claude client will also check env vars and OAuth)
	apiKey := viper.GetString("llm.api_key")
	oauthToken := viper.GetString("llm.oauth_token")

	cfg := llm.Config{
		Provider:   llm.Provider(provider),
		APIKey:     apiKey,
		OAuthToken: oauthToken,
		Model:      model,
		BaseURL:    baseURL,
	}

	if cfg.Provider == "" {
		cfg.Provider = llm.ProviderClaude
	}

	return llm.NewClient(cfg)
}

func runImportNotes(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Get LLM client
	llmClient, err := getLLMClient()
	if err != nil {
		return fmt.Errorf("failed to create LLM client: %w\n(Set ANTHROPIC_API_KEY environment variable or configure in config.yaml)", err)
	}

	// Parse duration
	var since time.Duration
	if strings.HasSuffix(importSinceFlag, "d") {
		days := strings.TrimSuffix(importSinceFlag, "d")
		var d int
		fmt.Sscanf(days, "%d", &d)
		since = time.Duration(d) * 24 * time.Hour
	} else {
		since, err = time.ParseDuration(importSinceFlag)
		if err != nil {
			return fmt.Errorf("invalid duration: %s", importSinceFlag)
		}
	}

	fmt.Println(titleStyle.Render("\n  Import from Apple Notes\n"))
	fmt.Printf("Looking for notes modified in the last %s...\n\n", importSinceFlag)

	// Read Apple Notes
	reader := apple_notes.NewReader()
	var notes []apple_notes.Note

	if importFolderFlag != "" {
		notes, err = reader.ListNotesByFolder(ctx, importFolderFlag)
	} else {
		notes, err = reader.ListRecentNotes(ctx, since)
	}

	if err != nil {
		return fmt.Errorf("failed to read Apple Notes: %w", err)
	}

	if len(notes) == 0 {
		fmt.Println("No notes found matching criteria.")
		return nil
	}

	fmt.Printf("Found %d note(s)\n\n", len(notes))

	return processNotes(ctx, llmClient, notesToGeneric(notes))
}

func runImportObsidian(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Get vault path
	vaultPath := importVaultFlag
	if len(args) > 0 {
		vaultPath = args[0]
	}
	if vaultPath == "" {
		vaultPath = viper.GetString("integrations.obsidian.vault_path")
	}
	if vaultPath == "" {
		return fmt.Errorf("vault path required: provide as argument or set in config")
	}

	// Get LLM client
	llmClient, err := getLLMClient()
	if err != nil {
		return fmt.Errorf("failed to create LLM client: %w", err)
	}

	fmt.Println(titleStyle.Render("\n  Import from Obsidian\n"))
	fmt.Printf("Reading from: %s\n\n", vaultPath)

	// Read Obsidian vault
	reader, err := obsidian.NewReader(vaultPath)
	if err != nil {
		return fmt.Errorf("failed to open vault: %w", err)
	}

	var notes []obsidian.Note

	if importSinceFlag != "" {
		since, err := parseDuration(importSinceFlag)
		if err != nil {
			return err
		}
		notes, err = reader.ListRecentNotes(ctx, since)
		if err != nil {
			return fmt.Errorf("failed to read notes: %w", err)
		}
	} else if importFolderFlag != "" {
		notes, err = reader.ListNotesByFolder(ctx, importFolderFlag)
		if err != nil {
			return fmt.Errorf("failed to read notes: %w", err)
		}
	} else {
		notes, err = reader.ListNotes(ctx)
		if err != nil {
			return fmt.Errorf("failed to read notes: %w", err)
		}
	}

	if len(notes) == 0 {
		fmt.Println("No notes found matching criteria.")
		return nil
	}

	fmt.Printf("Found %d note(s)\n\n", len(notes))

	return processNotes(ctx, llmClient, obsidianNotesToGeneric(notes))
}

func runImportInbox(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	inboxDir := filepath.Join(dataDir, "inbox")
	if _, err := os.Stat(inboxDir); os.IsNotExist(err) {
		fmt.Println("Inbox is empty.")
		return nil
	}

	// Get LLM client
	llmClient, err := getLLMClient()
	if err != nil {
		return fmt.Errorf("failed to create LLM client: %w", err)
	}

	fmt.Println(titleStyle.Render("\n  Process Inbox\n"))

	// Read inbox files
	reader, err := obsidian.NewReader(inboxDir)
	if err != nil {
		return fmt.Errorf("failed to read inbox: %w", err)
	}

	notes, err := reader.ListNotes(ctx)
	if err != nil {
		return fmt.Errorf("failed to read inbox notes: %w", err)
	}

	if len(notes) == 0 {
		fmt.Println("Inbox is empty.")
		return nil
	}

	fmt.Printf("Found %d item(s) in inbox\n\n", len(notes))

	return processNotes(ctx, llmClient, obsidianNotesToGeneric(notes))
}

// genericNote is a common format for notes from different sources
type genericNote struct {
	Name    string
	Content string
	Source  string
}

func notesToGeneric(notes []apple_notes.Note) []genericNote {
	result := make([]genericNote, len(notes))
	for i, n := range notes {
		result[i] = genericNote{
			Name:    n.Name,
			Content: n.PlainText,
			Source:  "apple_notes",
		}
	}
	return result
}

func obsidianNotesToGeneric(notes []obsidian.Note) []genericNote {
	result := make([]genericNote, len(notes))
	for i, n := range notes {
		result[i] = genericNote{
			Name:    n.Name,
			Content: n.Content,
			Source:  "obsidian",
		}
	}
	return result
}

func processNotes(ctx context.Context, llmClient llm.Client, notes []genericNote) error {
	reader := bufio.NewReader(os.Stdin)
	headerStyle := lipgloss.NewStyle().Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	for i, note := range notes {
		fmt.Printf("%s (%d/%d)\n", headerStyle.Render(note.Name), i+1, len(notes))

		// Preview content
		preview := note.Content
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		fmt.Println(labelStyle.Render(preview))
		fmt.Println()

		// Categorize with LLM
		fmt.Println("Analyzing...")
		result, err := llmClient.Categorize(ctx, note.Content)
		if err != nil {
			fmt.Printf("  Error: %v\n", err)
			continue
		}

		// Show categorization
		fmt.Printf("  %s %s (%.0f%% confidence)\n", labelStyle.Render("Area:"), result.Area, result.AreaConfidence*100)
		if result.ProjectSuggestion != "" {
			fmt.Printf("  %s %s\n", labelStyle.Render("Project:"), result.ProjectSuggestion)
		}
		if len(result.Tags) > 0 {
			fmt.Printf("  %s %s\n", labelStyle.Render("Tags:"), strings.Join(result.Tags, ", "))
		}
		fmt.Printf("  %s %s\n", labelStyle.Render("Summary:"), result.Summary)
		fmt.Printf("  %s %v\n", labelStyle.Render("Actionable:"), result.IsActionable)
		fmt.Println()

		if importDryRunFlag {
			fmt.Println(dimStyle.Render("  [Dry run - no changes made]"))
			fmt.Println()
			continue
		}

		// Confirm or auto-accept
		if !importAutoFlag {
			fmt.Print("Accept categorization? [Y/n/s(kip)]: ")
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(strings.ToLower(input))

			if input == "s" || input == "skip" {
				fmt.Println(dimStyle.Render("  Skipped"))
				fmt.Println()
				continue
			}
			if input != "" && input != "y" && input != "yes" {
				fmt.Println(dimStyle.Render("  Skipped"))
				fmt.Println()
				continue
			}
		}

		// Create project/tasks
		if err := createFromCategorization(ctx, note, result, llmClient); err != nil {
			fmt.Printf("  Error: %v\n", err)
		} else {
			fmt.Println(successStyle.Render("  ✓ Imported"))
		}
		fmt.Println()
	}

	return nil
}

func createFromCategorization(ctx context.Context, note genericNote, cat *llm.CategorizeResult, llmClient llm.Client) error {
	// Find or create area
	areas, err := client.ListAreas(ctx)
	if err != nil {
		return err
	}

	var targetArea *domain.Area
	for _, a := range areas {
		if strings.EqualFold(a.Slug(), cat.Area) || strings.EqualFold(a.Title, cat.Area) {
			targetArea = a
			break
		}
	}

	if targetArea == nil {
		// Create the area
		newArea := domain.NewArea(strings.Title(cat.Area))
		targetArea, err = client.CreateArea(ctx, newArea)
		if err != nil {
			return fmt.Errorf("failed to create area: %w", err)
		}
	}

	// Determine project name
	projectTitle := cat.ProjectSuggestion
	if projectTitle == "" {
		projectTitle = note.Name
	}

	// Check if project exists or create it
	projects, _ := client.ListProjects(ctx, targetArea.ID)
	var targetProject *domain.Project
	for _, p := range projects {
		if strings.EqualFold(p.Slug(), slugify(projectTitle)) {
			targetProject = p
			break
		}
	}

	if targetProject == nil {
		newProject := domain.NewProject(projectTitle, targetArea.ID)
		newProject.Content = cat.Summary
		for _, tag := range cat.Tags {
			newProject.AddTag(tag)
		}
		targetProject, err = client.CreateProject(ctx, newProject)
		if err != nil {
			return fmt.Errorf("failed to create project: %w", err)
		}
	}

	// Extract and create tasks if actionable
	if cat.IsActionable {
		tasks, err := llmClient.ExtractTasks(ctx, note.Content)
		if err != nil {
			return fmt.Errorf("failed to extract tasks: %w", err)
		}

		for _, t := range tasks {
			task := domain.NewTask(t.Title, targetProject.ID, targetArea.ID)
			task.Content = t.Description
			for _, tag := range t.Tags {
				task.AddTag(tag)
			}

			switch strings.ToLower(t.Priority) {
			case "low":
				task.Priority = domain.PriorityLow
			case "high":
				task.Priority = domain.PriorityHigh
			case "urgent":
				task.Priority = domain.PriorityUrgent
			default:
				task.Priority = domain.PriorityMedium
			}

			if _, err := client.CreateTask(ctx, task); err != nil {
				// Skip duplicate tasks
				continue
			}
		}
	}

	return nil
}

func parseDuration(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		days := strings.TrimSuffix(s, "d")
		var d int
		fmt.Sscanf(days, "%d", &d)
		return time.Duration(d) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

func slugify(s string) string {
	slug := strings.ToLower(s)
	slug = strings.ReplaceAll(slug, " ", "-")
	var result strings.Builder
	for _, r := range slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		}
	}
	return result.String()
}
