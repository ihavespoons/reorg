package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Project represents a collection of related tasks within an area
type Project struct {
	ID       string            `yaml:"id"`
	Title    string            `yaml:"title"`
	Type     string            `yaml:"type"`
	AreaID   string            `yaml:"area_id"`
	Status   ProjectStatus     `yaml:"status"`
	DueDate  *time.Time        `yaml:"due_date,omitempty"`
	Priority Priority          `yaml:"priority"`
	Tags     []string          `yaml:"tags,omitempty"`
	Metadata map[string]string `yaml:"metadata,omitempty"`
	Timestamps

	// Content holds the markdown body (not stored in frontmatter)
	Content string `yaml:"-"`
}

// NewProject creates a new Project with generated ID and timestamps
func NewProject(title, areaID string) *Project {
	p := &Project{
		ID:       fmt.Sprintf("proj-%s", uuid.New().String()[:8]),
		Title:    title,
		Type:     "project",
		AreaID:   areaID,
		Status:   ProjectStatusActive,
		Priority: PriorityMedium,
		Tags:     []string{},
		Metadata: make(map[string]string),
	}
	p.SetCreated()
	return p
}

// Slug returns a URL-safe identifier derived from the title
func (p *Project) Slug() string {
	slug := strings.ToLower(p.Title)
	slug = strings.ReplaceAll(slug, " ", "-")
	var result strings.Builder
	for _, r := range slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// Validate checks if the project has all required fields
func (p *Project) Validate() error {
	if p.ID == "" {
		return fmt.Errorf("project ID is required")
	}
	if p.Title == "" {
		return fmt.Errorf("project title is required")
	}
	if p.Type != "project" {
		return fmt.Errorf("project type must be 'project', got '%s'", p.Type)
	}
	if p.AreaID == "" {
		return fmt.Errorf("project area_id is required")
	}
	return nil
}

// IsActive returns true if the project is in an active state
func (p *Project) IsActive() bool {
	return p.Status == ProjectStatusActive
}

// IsComplete returns true if the project is completed
func (p *Project) IsComplete() bool {
	return p.Status == ProjectStatusCompleted
}

// Complete marks the project as completed
func (p *Project) Complete() {
	p.Status = ProjectStatusCompleted
	p.UpdateTimestamp()
}

// Archive marks the project as archived
func (p *Project) Archive() {
	p.Status = ProjectStatusArchived
	p.UpdateTimestamp()
}

// AddTag adds a tag if it doesn't already exist
func (p *Project) AddTag(tag string) {
	tag = strings.ToLower(strings.TrimSpace(tag))
	for _, t := range p.Tags {
		if t == tag {
			return
		}
	}
	p.Tags = append(p.Tags, tag)
	p.UpdateTimestamp()
}

// RemoveTag removes a tag if it exists
func (p *Project) RemoveTag(tag string) {
	tag = strings.ToLower(strings.TrimSpace(tag))
	for i, t := range p.Tags {
		if t == tag {
			p.Tags = append(p.Tags[:i], p.Tags[i+1:]...)
			p.UpdateTimestamp()
			return
		}
	}
}

// HasTag returns true if the project has the specified tag
func (p *Project) HasTag(tag string) bool {
	tag = strings.ToLower(strings.TrimSpace(tag))
	for _, t := range p.Tags {
		if t == tag {
			return true
		}
	}
	return false
}
