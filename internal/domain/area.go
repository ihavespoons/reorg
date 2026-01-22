package domain

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// Area represents a high-level category for organizing projects
// Examples: Work, Personal, Life Admin
type Area struct {
	ID        string            `yaml:"id"`
	Title     string            `yaml:"title"`
	Type      string            `yaml:"type"`
	Color     string            `yaml:"color,omitempty"`
	Icon      string            `yaml:"icon,omitempty"`
	SortOrder int               `yaml:"sort_order"`
	Metadata  map[string]string `yaml:"metadata,omitempty"`
	Timestamps

	// Content holds the markdown body (not stored in frontmatter)
	Content string `yaml:"-"`
}

// NewArea creates a new Area with generated ID and timestamps
func NewArea(title string) *Area {
	a := &Area{
		ID:        fmt.Sprintf("area-%s", uuid.New().String()[:8]),
		Title:     title,
		Type:      "area",
		SortOrder: 0,
		Metadata:  make(map[string]string),
	}
	a.SetCreated()
	return a
}

// Slug returns a URL-safe identifier derived from the title
func (a *Area) Slug() string {
	slug := strings.ToLower(a.Title)
	slug = strings.ReplaceAll(slug, " ", "-")
	// Remove any characters that aren't alphanumeric or hyphens
	var result strings.Builder
	for _, r := range slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// Validate checks if the area has all required fields
func (a *Area) Validate() error {
	if a.ID == "" {
		return fmt.Errorf("area ID is required")
	}
	if a.Title == "" {
		return fmt.Errorf("area title is required")
	}
	if a.Type != "area" {
		return fmt.Errorf("area type must be 'area', got '%s'", a.Type)
	}
	return nil
}

// DefaultAreas returns the suggested default areas for interactive init
func DefaultAreas() []*Area {
	work := NewArea("Work")
	work.Icon = "briefcase"
	work.Color = "#4A90D9"
	work.SortOrder = 1
	work.Content = "All work-related projects and tasks."

	personal := NewArea("Personal")
	personal.Icon = "user"
	personal.Color = "#7ED321"
	personal.SortOrder = 2
	personal.Content = "Personal projects and goals."

	lifeAdmin := NewArea("Life Admin")
	lifeAdmin.Icon = "clipboard"
	lifeAdmin.Color = "#F5A623"
	lifeAdmin.SortOrder = 3
	lifeAdmin.Content = "Administrative tasks: bills, appointments, paperwork, etc."

	return []*Area{work, personal, lifeAdmin}
}
