package obsidian

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/adrg/frontmatter"
)

// Note represents a markdown note from an Obsidian vault or similar
type Note struct {
	Path         string            `json:"path"`
	Name         string            `json:"name"`
	Content      string            `json:"content"`
	Frontmatter  map[string]any    `json:"frontmatter,omitempty"`
	Tags         []string          `json:"tags,omitempty"`
	Links        []string          `json:"links,omitempty"`
	ModTime      time.Time         `json:"mod_time"`
	RelativePath string            `json:"relative_path"`
}

// Reader reads markdown notes from a directory
type Reader struct {
	rootDir string
}

// NewReader creates a new markdown reader for the given directory
func NewReader(rootDir string) (*Reader, error) {
	// Expand ~ if present
	if strings.HasPrefix(rootDir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		rootDir = filepath.Join(home, rootDir[2:])
	}

	// Verify directory exists
	info, err := os.Stat(rootDir)
	if err != nil {
		return nil, fmt.Errorf("vault directory not found: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("path is not a directory: %s", rootDir)
	}

	return &Reader{rootDir: rootDir}, nil
}

// RootDir returns the root directory
func (r *Reader) RootDir() string {
	return r.rootDir
}

// ListNotes returns all markdown notes in the vault
func (r *Reader) ListNotes(ctx context.Context) ([]Note, error) {
	var notes []Note

	err := filepath.WalkDir(r.rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip hidden directories
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") {
			return fs.SkipDir
		}

		// Only process markdown files
		if d.IsDir() || !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}

		note, err := r.ReadNote(ctx, path)
		if err != nil {
			// Log but don't fail on individual file errors
			return nil
		}

		notes = append(notes, *note)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk vault: %w", err)
	}

	return notes, nil
}

// ReadNote reads a single markdown note
func (r *Reader) ReadNote(ctx context.Context, path string) (*Note, error) {
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
		// No frontmatter, use full content
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
			tags = strings.Split(t, ",")
			for i := range tags {
				tags[i] = strings.TrimSpace(tags[i])
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

	// Calculate relative path
	relPath, _ := filepath.Rel(r.rootDir, path)

	return &Note{
		Path:         path,
		Name:         strings.TrimSuffix(filepath.Base(path), ".md"),
		Content:      string(body),
		Frontmatter:  fm,
		Tags:         tags,
		Links:        links,
		ModTime:      info.ModTime(),
		RelativePath: relPath,
	}, nil
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
		if note.ModTime.After(cutoff) {
			recent = append(recent, note)
		}
	}

	return recent, nil
}

// ListNotesByTag returns notes with a specific tag
func (r *Reader) ListNotesByTag(ctx context.Context, tag string) ([]Note, error) {
	notes, err := r.ListNotes(ctx)
	if err != nil {
		return nil, err
	}

	tag = strings.ToLower(strings.TrimPrefix(tag, "#"))
	var filtered []Note
	for _, note := range notes {
		for _, t := range note.Tags {
			if strings.ToLower(strings.TrimPrefix(t, "#")) == tag {
				filtered = append(filtered, note)
				break
			}
		}
	}

	return filtered, nil
}

// ListNotesByFolder returns notes in a specific folder (relative path)
func (r *Reader) ListNotesByFolder(ctx context.Context, folder string) ([]Note, error) {
	notes, err := r.ListNotes(ctx)
	if err != nil {
		return nil, err
	}

	folder = strings.ToLower(folder)
	var filtered []Note
	for _, note := range notes {
		noteDir := strings.ToLower(filepath.Dir(note.RelativePath))
		if strings.HasPrefix(noteDir, folder) || noteDir == folder {
			filtered = append(filtered, note)
		}
	}

	return filtered, nil
}

// extractInlineTags finds #tags in text
func extractInlineTags(text string) []string {
	var tags []string
	words := strings.Fields(text)
	for _, word := range words {
		if strings.HasPrefix(word, "#") && len(word) > 1 {
			// Clean up the tag
			tag := strings.TrimPrefix(word, "#")
			tag = strings.TrimRight(tag, ".,;:!?)")
			if tag != "" && !contains(tags, tag) {
				tags = append(tags, tag)
			}
		}
	}
	return tags
}

// extractWikilinks finds [[links]] in text
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
		// Handle [[link|alias]] format
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

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
