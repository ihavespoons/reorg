// Plugin binary for importing Obsidian notes into reorg.
//
// This plugin reads markdown files from an Obsidian vault and imports them
// as projects and tasks in reorg.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/adrg/frontmatter"

	"github.com/ihavespoons/reorg/pkg/plugin"
)

func main() {
	plugin.Serve(&ObsidianPlugin{})
}

// ObsidianPlugin implements the plugin.Plugin interface for Obsidian.
type ObsidianPlugin struct {
	host     plugin.HostClient
	config   Config
	stateDir string
}

// Config holds plugin configuration.
type Config struct {
	VaultPath string   `json:"vault_path"`
	Since     string   `json:"since"`     // Duration like "24h" or "7d"
	Folders   []string `json:"folders"`   // Only import from these folders
	SkipDirs  []string `json:"skip_dirs"` // Skip these directories
}

// State tracks what has been imported.
type State struct {
	LastRun        time.Time         `json:"last_run"`
	ProcessedNotes map[string]string `json:"processed_notes"` // file path -> content hash
}

// Note represents a markdown note from Obsidian.
type Note struct {
	Path         string
	Name         string
	Content      string
	Frontmatter  map[string]any
	Tags         []string
	Links        []string
	ModTime      time.Time
	RelativePath string
}

func (p *ObsidianPlugin) GetManifest(ctx context.Context) (*plugin.Manifest, error) {
	return &plugin.Manifest{
		Name:        "obsidian-importer",
		Version:     "1.0.0",
		Description: "Import notes from Obsidian vault into reorg",
		Author:      "reorg",
		Schedule:    "0 0 */6 * * *", // Every 6 hours (with seconds)
		Capabilities: []string{"import"},
	}, nil
}

func (p *ObsidianPlugin) Configure(ctx context.Context, host plugin.HostClient, config map[string]string, stateDir string) error {
	p.host = host
	p.stateDir = stateDir

	// Parse configuration
	p.config = Config{
		VaultPath: "",
		Since:     "24h",
		SkipDirs:  []string{".obsidian", ".git", ".trash"},
	}

	if vaultPath, ok := config["vault_path"]; ok {
		p.config.VaultPath = vaultPath
	}
	if since, ok := config["since"]; ok {
		p.config.Since = since
	}
	if folders, ok := config["folders"]; ok && folders != "" {
		p.config.Folders = strings.Split(folders, ",")
		for i := range p.config.Folders {
			p.config.Folders[i] = strings.TrimSpace(p.config.Folders[i])
		}
	}
	if skipDirs, ok := config["skip_dirs"]; ok && skipDirs != "" {
		p.config.SkipDirs = strings.Split(skipDirs, ",")
		for i := range p.config.SkipDirs {
			p.config.SkipDirs[i] = strings.TrimSpace(p.config.SkipDirs[i])
		}
	}

	// Expand ~ in vault path
	if p.config.VaultPath != "" && strings.HasPrefix(p.config.VaultPath, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			p.config.VaultPath = filepath.Join(home, p.config.VaultPath[2:])
		}
	}

	return nil
}

func (p *ObsidianPlugin) Execute(ctx context.Context, params *plugin.ExecuteParams) (*plugin.ExecuteResult, error) {
	result := &plugin.ExecuteResult{
		Success: true,
		Summary: &plugin.ExecuteSummary{},
	}

	if p.config.VaultPath == "" {
		result.Success = false
		result.Error = "vault_path not configured"
		return result, nil
	}

	// Check vault exists
	if _, err := os.Stat(p.config.VaultPath); os.IsNotExist(err) {
		result.Success = false
		result.Error = fmt.Sprintf("vault not found: %s", p.config.VaultPath)
		return result, nil
	}

	// Load state
	state := &State{
		ProcessedNotes: make(map[string]string),
	}
	stateHelper := plugin.NewStateJSON(p.host)
	_, _ = stateHelper.Get(ctx, "state", state)

	// Parse duration
	var since time.Duration
	var err error
	if p.config.Since != "" {
		since, err = parseDuration(p.config.Since)
		if err != nil {
			result.Success = false
			result.Error = fmt.Sprintf("invalid since duration: %v", err)
			return result, nil
		}
	}

	// Read notes from vault
	notes, err := p.readNotes(ctx, since)
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("failed to read vault: %v", err)
		return result, nil
	}

	result.Summary.ItemsProcessed = len(notes)

	// Build project context for AI matching
	projectContext, _ := p.host.BuildProjectContext(ctx)

	// Process each note
	for _, note := range notes {
		// Check if already processed (by content hash)
		hash := hashContent(note.Content)
		if existingHash, exists := state.ProcessedNotes[note.RelativePath]; exists && existingHash == hash {
			result.Summary.ItemsSkipped++
			result.Results = append(result.Results, plugin.ExecuteItem{
				ID:      note.RelativePath,
				Name:    note.Name,
				Action:  "skipped",
				Message: "already imported (unchanged)",
			})
			continue
		}

		// Dry run - just track what would be imported
		if params.DryRun {
			result.Summary.ItemsImported++
			result.Results = append(result.Results, plugin.ExecuteItem{
				ID:     note.RelativePath,
				Name:   note.Name,
				Action: "imported",
				Metadata: map[string]string{
					"path": note.RelativePath,
				},
			})
			continue
		}

		// Import the note
		if err := p.importNote(ctx, note, projectContext); err != nil {
			result.Summary.ItemsFailed++
			result.Results = append(result.Results, plugin.ExecuteItem{
				ID:      note.RelativePath,
				Name:    note.Name,
				Action:  "failed",
				Message: err.Error(),
			})
			continue
		}

		// Mark as processed
		state.ProcessedNotes[note.RelativePath] = hash
		result.Summary.ItemsImported++
		result.Results = append(result.Results, plugin.ExecuteItem{
			ID:     note.RelativePath,
			Name:   note.Name,
			Action: "imported",
			Metadata: map[string]string{
				"path": note.RelativePath,
			},
		})
	}

	// Save state
	if !params.DryRun {
		state.LastRun = time.Now()
		_ = stateHelper.Set(ctx, "state", state)
	}

	result.Summary.Message = fmt.Sprintf("Processed %d notes from Obsidian vault", len(notes))
	return result, nil
}

func (p *ObsidianPlugin) Shutdown(ctx context.Context) error {
	return nil
}

func (p *ObsidianPlugin) importNote(ctx context.Context, note Note, projectContext []plugin.ProjectContext) error {
	// Use AI to categorize
	catResult, err := p.host.CategorizeWithContext(ctx, note.Content, projectContext)
	if err != nil {
		// If AI fails, use default categorization
		catResult = &plugin.CategorizeResult{
			Area:              "personal",
			AreaConfidence:    0.5,
			ProjectSuggestion: note.Name,
			Summary:           truncate(note.Content, 200),
			IsActionable:      false,
			Tags:              note.Tags,
		}
	}

	// Find or create area
	area, err := p.host.FindOrCreateArea(ctx, catResult.Area)
	if err != nil {
		return fmt.Errorf("failed to find/create area: %w", err)
	}

	// Find or create project
	var project *plugin.Project
	if catResult.ProjectID != "" {
		project, err = p.host.GetProject(ctx, catResult.ProjectID)
		if err != nil {
			project = nil
		}
	}

	if project == nil {
		projectTitle := catResult.ProjectSuggestion
		if projectTitle == "" {
			projectTitle = note.Name
		}

		// Merge tags from note and categorization
		tags := catResult.Tags
		for _, t := range note.Tags {
			if !contains(tags, t) {
				tags = append(tags, t)
			}
		}

		project, err = p.host.FindOrCreateProject(ctx, projectTitle, area.ID, catResult.Summary, tags)
		if err != nil {
			return fmt.Errorf("failed to find/create project: %w", err)
		}
	}

	// Extract and create tasks if actionable
	if catResult.IsActionable {
		tasks, err := p.host.ExtractTasks(ctx, note.Content)
		if err == nil && len(tasks) > 0 {
			for _, t := range tasks {
				priority := plugin.PriorityMedium
				switch strings.ToLower(t.Priority) {
				case "low":
					priority = plugin.PriorityLow
				case "high":
					priority = plugin.PriorityHigh
				case "urgent":
					priority = plugin.PriorityUrgent
				}

				_, _ = p.host.CreateTask(ctx, t.Title, project.ID, area.ID, t.Description, priority, t.Tags)
			}
		}
	}

	return nil
}

func (p *ObsidianPlugin) readNotes(ctx context.Context, since time.Duration) ([]Note, error) {
	var notes []Note
	cutoff := time.Now().Add(-since)

	err := filepath.WalkDir(p.config.VaultPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip hidden directories and configured skip dirs
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") {
				return fs.SkipDir
			}
			for _, skipDir := range p.config.SkipDirs {
				if name == skipDir {
					return fs.SkipDir
				}
			}
			return nil
		}

		// Only process markdown files
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}

		// Get file info for mod time
		info, err := d.Info()
		if err != nil {
			return nil
		}

		// Filter by modification time
		if since > 0 && info.ModTime().Before(cutoff) {
			return nil
		}

		// Calculate relative path
		relPath, _ := filepath.Rel(p.config.VaultPath, path)

		// Filter by folders if configured
		if len(p.config.Folders) > 0 {
			dir := filepath.Dir(relPath)
			matched := false
			for _, folder := range p.config.Folders {
				if strings.HasPrefix(dir, folder) || dir == folder || dir == "." && folder == "" {
					matched = true
					break
				}
			}
			if !matched {
				return nil
			}
		}

		// Read and parse the note
		note, err := p.readNote(ctx, path)
		if err != nil {
			return nil // Skip files that can't be read
		}

		note.RelativePath = relPath
		notes = append(notes, *note)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk vault: %w", err)
	}

	return notes, nil
}

func (p *ObsidianPlugin) readNote(ctx context.Context, path string) (*Note, error) {
	// Get file info
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	// Read file content
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	content := string(data)

	// Parse frontmatter if present
	var fm map[string]any
	reader := strings.NewReader(content)
	body, err := frontmatter.Parse(reader, &fm)
	if err != nil {
		body = data
	}

	// Extract tags from frontmatter
	var tags []string
	if fmTags, ok := fm["tags"]; ok {
		switch t := fmTags.(type) {
		case []any:
			for _, tag := range t {
				if s, ok := tag.(string); ok {
					tags = append(tags, s)
				}
			}
		case []string:
			tags = t
		case string:
			parts := strings.Split(t, ",")
			for _, part := range parts {
				tags = append(tags, strings.TrimSpace(part))
			}
		}
	}

	// Extract inline tags (#tag)
	inlineTags := extractInlineTags(string(body))
	for _, tag := range inlineTags {
		if !contains(tags, tag) {
			tags = append(tags, tag)
		}
	}

	// Extract wikilinks [[link]]
	links := extractWikilinks(string(body))

	return &Note{
		Path:        path,
		Name:        strings.TrimSuffix(filepath.Base(path), ".md"),
		Content:     string(body),
		Frontmatter: fm,
		Tags:        tags,
		Links:       links,
		ModTime:     info.ModTime(),
	}, nil
}

// Helper functions

func parseDuration(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		days := strings.TrimSuffix(s, "d")
		var d int
		_, _ = fmt.Sscanf(days, "%d", &d)
		return time.Duration(d) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

func extractInlineTags(text string) []string {
	var tags []string
	words := strings.Fields(text)
	for _, word := range words {
		if strings.HasPrefix(word, "#") && len(word) > 1 {
			tag := strings.TrimPrefix(word, "#")
			tag = strings.TrimRight(tag, ".,;:!?)")
			if tag != "" && !contains(tags, tag) {
				tags = append(tags, tag)
			}
		}
	}
	return tags
}

func extractWikilinks(text string) []string {
	var links []string
	for {
		start := strings.Index(text, "[[")
		if start == -1 {
			break
		}
		end := strings.Index(text[start:], "]]")
		if end == -1 {
			break
		}

		link := text[start+2 : start+end]
		if pipe := strings.Index(link, "|"); pipe != -1 {
			link = link[:pipe]
		}
		if link != "" && !contains(links, link) {
			links = append(links, link)
		}

		text = text[start+end+2:]
	}
	return links
}

func hashContent(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:8])
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// Ensure interface is satisfied
var _ plugin.Plugin = (*ObsidianPlugin)(nil)
