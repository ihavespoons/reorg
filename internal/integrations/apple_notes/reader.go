package apple_notes

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Note represents an Apple Note
type Note struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Body         string    `json:"body"`
	PlainText    string    `json:"plain_text"`
	CreationDate time.Time `json:"creation_date"`
	ModDate      time.Time `json:"modification_date"`
	Folder       string    `json:"folder"`
}

// Reader reads notes from Apple Notes via AppleScript
type Reader struct{}

// NewReader creates a new Apple Notes reader
func NewReader() *Reader {
	return &Reader{}
}

// ListNotes returns all notes from Apple Notes
func (r *Reader) ListNotes(ctx context.Context) ([]Note, error) {
	// AppleScript to get all notes with their details
	script := `
tell application "Notes"
	set noteList to ""
	repeat with n in notes
		set noteID to id of n
		set noteName to name of n
		set noteBody to body of n
		set noteCreation to creation date of n
		set noteMod to modification date of n
		set noteFolder to name of container of n

		-- Escape special characters for JSON
		set noteName to my escapeForJSON(noteName)
		set noteBody to my escapeForJSON(noteBody)
		set noteFolder to my escapeForJSON(noteFolder)

		set noteJSON to "{\"id\":\"" & noteID & "\",\"name\":\"" & noteName & "\",\"body\":\"" & noteBody & "\",\"folder\":\"" & noteFolder & "\",\"creation_date\":\"" & (noteCreation as «class isot» as string) & "\",\"modification_date\":\"" & (noteMod as «class isot» as string) & "\"}"

		if noteList is "" then
			set noteList to noteJSON
		else
			set noteList to noteList & "," & noteJSON
		end if
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
`

	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("osascript error: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("failed to execute osascript: %w", err)
	}

	var notes []Note
	if err := json.Unmarshal(output, &notes); err != nil {
		return nil, fmt.Errorf("failed to parse notes: %w (output: %s)", err, string(output))
	}

	// Extract plain text from HTML body
	for i := range notes {
		notes[i].PlainText = stripHTML(notes[i].Body)
	}

	return notes, nil
}

// ListRecentNotes returns notes modified within the given duration
func (r *Reader) ListRecentNotes(ctx context.Context, since time.Duration) ([]Note, error) {
	notes, err := r.ListNotes(ctx)
	if err != nil {
		return nil, err
	}

	cutoff := time.Now().Add(-since)
	var recent []Note
	for _, note := range notes {
		if note.ModDate.After(cutoff) {
			recent = append(recent, note)
		}
	}

	return recent, nil
}

// ListNotesByFolder returns notes from a specific folder
func (r *Reader) ListNotesByFolder(ctx context.Context, folder string) ([]Note, error) {
	notes, err := r.ListNotes(ctx)
	if err != nil {
		return nil, err
	}

	folder = strings.ToLower(folder)
	var filtered []Note
	for _, note := range notes {
		if strings.ToLower(note.Folder) == folder {
			filtered = append(filtered, note)
		}
	}

	return filtered, nil
}

// GetNote returns a specific note by ID
func (r *Reader) GetNote(ctx context.Context, id string) (*Note, error) {
	notes, err := r.ListNotes(ctx)
	if err != nil {
		return nil, err
	}

	for _, note := range notes {
		if note.ID == id {
			return &note, nil
		}
	}

	return nil, fmt.Errorf("note not found: %s", id)
}

// ListFolders returns all folder names in Apple Notes
func (r *Reader) ListFolders(ctx context.Context) ([]string, error) {
	script := `
tell application "Notes"
	set folderNames to {}
	repeat with f in folders
		set end of folderNames to name of f
	end repeat
	return folderNames
end tell
`

	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list folders: %w", err)
	}

	// Parse AppleScript list output (comma-separated)
	result := strings.TrimSpace(string(output))
	if result == "" {
		return []string{}, nil
	}

	folders := strings.Split(result, ", ")
	return folders, nil
}

// stripHTML removes HTML tags from a string
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
	// Clean up extra whitespace
	text = strings.ReplaceAll(text, "&nbsp;", " ")
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&quot;", "\"")

	// Normalize whitespace
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
