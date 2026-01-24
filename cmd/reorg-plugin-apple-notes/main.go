// Plugin binary for importing Apple Notes into reorg.
//
// This plugin reads notes from Apple Notes via AppleScript and imports them
// as projects and tasks in reorg.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/ihavespoons/reorg/pkg/plugin"
)

func main() {
	plugin.Serve(&AppleNotesPlugin{})
}

// AppleNotesPlugin implements the plugin.Plugin interface for Apple Notes.
type AppleNotesPlugin struct {
	host     plugin.HostClient
	config   Config
	stateDir string
}

// Config holds plugin configuration.
type Config struct {
	Since   string   `json:"since"`   // Duration like "24h" or "7d"
	Auto    bool     `json:"auto"`    // Auto-accept categorizations
	Folders []string `json:"folders"` // Only import from these folders
}

// State tracks what has been imported.
type State struct {
	LastRun        time.Time         `json:"last_run"`
	ProcessedNotes map[string]string `json:"processed_notes"` // note ID -> content hash
}

// Note represents an Apple Note.
type Note struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Body         string    `json:"body"`
	PlainText    string    `json:"plain_text"`
	Folder       string    `json:"folder"`
	CreationDate time.Time `json:"creation_date"`
	ModDate      time.Time `json:"modification_date"`
}

// noteJSON is used for JSON unmarshaling with string dates.
type noteJSON struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Body         string `json:"body"`
	Folder       string `json:"folder"`
	CreationDate string `json:"creation_date"`
	ModDate      string `json:"modification_date"`
}

func (p *AppleNotesPlugin) GetManifest(ctx context.Context) (*plugin.Manifest, error) {
	return &plugin.Manifest{
		Name:        "apple-notes-importer",
		Version:     "1.0.0",
		Description: "Import notes from Apple Notes into reorg",
		Author:      "reorg",
		Schedule:    "0 */15 * * * *", // Every 15 minutes (with seconds)
		Capabilities: []string{"import"},
	}, nil
}

func (p *AppleNotesPlugin) Configure(ctx context.Context, host plugin.HostClient, config map[string]string, stateDir string) error {
	p.host = host
	p.stateDir = stateDir

	// Parse configuration
	p.config = Config{
		Since: "24h",
		Auto:  true,
	}

	if since, ok := config["since"]; ok {
		p.config.Since = since
	}
	if auto, ok := config["auto"]; ok {
		p.config.Auto = auto == "true"
	}
	if folders, ok := config["folders"]; ok && folders != "" {
		p.config.Folders = strings.Split(folders, ",")
		for i := range p.config.Folders {
			p.config.Folders[i] = strings.TrimSpace(p.config.Folders[i])
		}
	}

	return nil
}

func (p *AppleNotesPlugin) Execute(ctx context.Context, params *plugin.ExecuteParams) (*plugin.ExecuteResult, error) {
	result := &plugin.ExecuteResult{
		Success: true,
		Summary: &plugin.ExecuteSummary{},
	}

	// Load state
	state := &State{
		ProcessedNotes: make(map[string]string),
	}
	stateHelper := plugin.NewStateJSON(p.host)
	_, _ = stateHelper.Get(ctx, "state", state)

	// Parse duration
	since, err := parseDuration(p.config.Since)
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("invalid since duration: %v", err)
		return result, nil
	}

	// Read notes from Apple Notes
	notes, err := p.readNotes(ctx, since)
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("failed to read Apple Notes: %v", err)
		return result, nil
	}

	// Filter by folders if configured
	if len(p.config.Folders) > 0 {
		var filtered []Note
		for _, note := range notes {
			for _, folder := range p.config.Folders {
				if strings.EqualFold(note.Folder, folder) {
					filtered = append(filtered, note)
					break
				}
			}
		}
		notes = filtered
	}

	result.Summary.ItemsProcessed = len(notes)

	// Build project context for AI matching
	projectContext, _ := p.host.BuildProjectContext(ctx)

	// Process each note
	for _, note := range notes {
		// Check if already processed (by content hash)
		hash := hashContent(note.PlainText)
		if existingHash, exists := state.ProcessedNotes[note.ID]; exists && existingHash == hash {
			result.Summary.ItemsSkipped++
			result.Results = append(result.Results, plugin.ExecuteItem{
				ID:      note.ID,
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
				ID:     note.ID,
				Name:   note.Name,
				Action: "imported",
				Metadata: map[string]string{
					"folder": note.Folder,
				},
			})
			continue
		}

		// Import the note
		if err := p.importNote(ctx, note, projectContext); err != nil {
			result.Summary.ItemsFailed++
			result.Results = append(result.Results, plugin.ExecuteItem{
				ID:      note.ID,
				Name:    note.Name,
				Action:  "failed",
				Message: err.Error(),
			})
			continue
		}

		// Mark as processed
		state.ProcessedNotes[note.ID] = hash
		result.Summary.ItemsImported++
		result.Results = append(result.Results, plugin.ExecuteItem{
			ID:     note.ID,
			Name:   note.Name,
			Action: "imported",
			Metadata: map[string]string{
				"folder": note.Folder,
			},
		})
	}

	// Save state
	if !params.DryRun {
		state.LastRun = time.Now()
		_ = stateHelper.Set(ctx, "state", state)
	}

	result.Summary.Message = fmt.Sprintf("Processed %d notes from Apple Notes", len(notes))
	return result, nil
}

func (p *AppleNotesPlugin) Shutdown(ctx context.Context) error {
	return nil
}

func (p *AppleNotesPlugin) importNote(ctx context.Context, note Note, projectContext []plugin.ProjectContext) error {
	// Use AI to categorize
	catResult, err := p.host.CategorizeWithContext(ctx, note.PlainText, projectContext)
	if err != nil {
		// If AI fails, use default categorization
		catResult = &plugin.CategorizeResult{
			Area:              "personal",
			AreaConfidence:    0.5,
			ProjectSuggestion: note.Name,
			Summary:           truncate(note.PlainText, 200),
			IsActionable:      false,
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
		// Use existing project
		project, err = p.host.GetProject(ctx, catResult.ProjectID)
		if err != nil {
			// Fall back to creating new
			project = nil
		}
	}

	if project == nil {
		projectTitle := catResult.ProjectSuggestion
		if projectTitle == "" {
			projectTitle = note.Name
		}

		project, err = p.host.FindOrCreateProject(ctx, projectTitle, area.ID, catResult.Summary, catResult.Tags)
		if err != nil {
			return fmt.Errorf("failed to find/create project: %w", err)
		}
	}

	// Extract and create tasks if actionable
	if catResult.IsActionable {
		tasks, err := p.host.ExtractTasks(ctx, note.PlainText)
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

func (p *AppleNotesPlugin) readNotes(ctx context.Context, since time.Duration) ([]Note, error) {
	secondsAgo := int(since.Seconds())

	script := fmt.Sprintf(`
tell application "Notes"
	set cutoffDate to (current date) - %d
	set noteList to ""

	try
		set recentNotes to every note whose modification date > cutoffDate
	on error
		set recentNotes to {}
		repeat with n in notes
			try
				if modification date of n > cutoffDate then
					set end of recentNotes to n
				end if
			end try
		end repeat
	end try

	repeat with n in recentNotes
		try
			set noteID to id of n
			set noteName to name of n
			set noteBody to body of n
			set noteCreation to creation date of n
			set noteMod to modification date of n

			set noteFolder to "Notes"
			try
				set noteFolder to name of container of n
			end try

			set noteName to my escapeForJSON(noteName)
			set noteBody to my escapeForJSON(noteBody)
			set escapedFolder to my escapeForJSON(noteFolder)

			set noteJSON to "{\"id\":\"" & noteID & "\",\"name\":\"" & noteName & "\",\"body\":\"" & noteBody & "\",\"folder\":\"" & escapedFolder & "\",\"creation_date\":\"" & (noteCreation as «class isot» as string) & "\",\"modification_date\":\"" & (noteMod as «class isot» as string) & "\"}"

			if noteList is "" then
				set noteList to noteJSON
			else
				set noteList to noteList & "," & noteJSON
			end if
		end try
	end repeat

	return "[" & noteList & "]"
end tell

on escapeForJSON(theText)
	set theText to my replaceText(theText, "\\", "\\\\")
	set theText to my replaceText(theText, "\"", "\\\"")
	set theText to my replaceText(theText, return, "\\n")
	set theText to my replaceText(theText, linefeed, "\\n")
	set theText to my replaceText(theText, tab, "\\t")
	return theText
end escapeForJSON

on replaceText(theText, searchString, replacementString)
	set AppleScript's text item delimiters to searchString
	set theTextItems to every text item of theText
	set AppleScript's text item delimiters to replacementString
	set theText to theTextItems as string
	set AppleScript's text item delimiters to ""
	return theText
end replaceText
`, secondsAgo)

	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("osascript error: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("failed to execute osascript: %w", err)
	}

	// Parse JSON
	var jsonNotes []noteJSON
	if err := json.Unmarshal(output, &jsonNotes); err != nil {
		return nil, fmt.Errorf("failed to parse notes: %w (output: %s)", err, string(output))
	}

	// Convert to Note structs
	notes := make([]Note, len(jsonNotes))
	for i, jn := range jsonNotes {
		notes[i] = Note{
			ID:           jn.ID,
			Name:         jn.Name,
			Body:         jn.Body,
			Folder:       jn.Folder,
			CreationDate: parseAppleScriptDate(jn.CreationDate),
			ModDate:      parseAppleScriptDate(jn.ModDate),
			PlainText:    stripHTML(jn.Body),
		}
	}

	return notes, nil
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

func parseAppleScriptDate(s string) time.Time {
	layouts := []string{
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:05Z07:00",
		time.RFC3339,
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t
		}
	}
	return time.Time{}
}

func stripHTML(html string) string {
	var result strings.Builder
	var inTag bool

	for _, r := range html {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			result.WriteRune(r)
		}
	}

	text := result.String()
	text = strings.ReplaceAll(text, "&nbsp;", " ")
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&quot;", "\"")

	lines := strings.Split(text, "\n")
	var cleanLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			cleanLines = append(cleanLines, line)
		}
	}

	return strings.Join(cleanLines, "\n")
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

// Ensure interface is satisfied
var _ plugin.Plugin = (*AppleNotesPlugin)(nil)
