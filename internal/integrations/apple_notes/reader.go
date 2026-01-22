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
	CreationDate time.Time `json:"-"`
	ModDate      time.Time `json:"-"`
	Folder       string    `json:"folder"`
}

// noteJSON is used for JSON unmarshaling with string dates
type noteJSON struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Body         string `json:"body"`
	Folder       string `json:"folder"`
	CreationDate string `json:"creation_date"`
	ModDate      string `json:"modification_date"`
}

// parseAppleScriptDate parses dates from AppleScript's ISO format (without timezone)
func parseAppleScriptDate(s string) time.Time {
	// AppleScript returns dates in local time without timezone: 2026-01-22T09:22:08
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

// Reader reads notes from Apple Notes via AppleScript
type Reader struct{}

// NewReader creates a new Apple Notes reader
func NewReader() *Reader {
	return &Reader{}
}

// ListNotes returns all notes from Apple Notes
func (r *Reader) ListNotes(ctx context.Context) ([]Note, error) {
	// AppleScript to get all notes by iterating through folders first
	// This is more reliable and gets folder names correctly
	script := `
tell application "Notes"
	set noteList to ""

	-- Iterate through all accounts and their folders
	repeat with acc in accounts
		repeat with fld in folders of acc
			set folderName to name of fld

			repeat with n in notes of fld
				try
					set noteID to id of n
					set noteName to name of n
					set noteBody to body of n
					set noteCreation to creation date of n
					set noteMod to modification date of n

					-- Escape special characters for JSON
					set noteName to my escapeForJSON(noteName)
					set noteBody to my escapeForJSON(noteBody)
					set escapedFolder to my escapeForJSON(folderName)

					set noteJSON to "{\"id\":\"" & noteID & "\",\"name\":\"" & noteName & "\",\"body\":\"" & noteBody & "\",\"folder\":\"" & escapedFolder & "\",\"creation_date\":\"" & (noteCreation as «class isot» as string) & "\",\"modification_date\":\"" & (noteMod as «class isot» as string) & "\"}"

					if noteList is "" then
						set noteList to noteJSON
					else
						set noteList to noteList & "," & noteJSON
					end if
				end try
			end repeat
		end repeat
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

	// Parse into intermediate struct with string dates
	var jsonNotes []noteJSON
	if err := json.Unmarshal(output, &jsonNotes); err != nil {
		return nil, fmt.Errorf("failed to parse notes: %w (output: %s)", err, string(output))
	}

	// Convert to Note structs with proper date parsing
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

// ListRecentNotes returns notes modified within the given duration
// This version uses AppleScript's 'whose' clause for potentially faster filtering
func (r *Reader) ListRecentNotes(ctx context.Context, since time.Duration) ([]Note, error) {
	// Calculate seconds ago for AppleScript (avoids locale-dependent date parsing)
	secondsAgo := int(since.Seconds())

	// AppleScript using 'whose' clause for filtering - may be faster if Notes supports it
	script := fmt.Sprintf(`
tell application "Notes"
	set cutoffDate to (current date) - %d
	set noteList to ""

	-- Try to get recent notes using whose clause (faster if supported)
	try
		set recentNotes to every note whose modification date > cutoffDate
	on error
		-- Fallback: get all notes and filter manually
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

			-- Get folder name
			set noteFolder to "Notes"
			try
				set noteFolder to name of container of n
			end try

			-- Escape special characters for JSON
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

	// Parse into intermediate struct with string dates
	var jsonNotes []noteJSON
	if err := json.Unmarshal(output, &jsonNotes); err != nil {
		return nil, fmt.Errorf("failed to parse notes: %w (output: %s)", err, string(output))
	}

	// Convert to Note structs with proper date parsing
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
