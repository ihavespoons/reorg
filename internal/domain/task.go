package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Task represents a single actionable item within a project
type Task struct {
	ID           string            `yaml:"id"`
	Title        string            `yaml:"title"`
	Type         string            `yaml:"type"`
	ProjectID    string            `yaml:"project_id"`
	AreaID       string            `yaml:"area_id"`
	Status       TaskStatus        `yaml:"status"`
	DueDate      *time.Time        `yaml:"due_date,omitempty"`
	Priority     Priority          `yaml:"priority"`
	Assignee     string            `yaml:"assignee,omitempty"`
	Tags         []string          `yaml:"tags,omitempty"`
	Dependencies []string          `yaml:"dependencies,omitempty"`
	TimeEstimate string            `yaml:"time_estimate,omitempty"`
	TimeSpent    string            `yaml:"time_spent,omitempty"`
	Recurrence   *string           `yaml:"recurrence,omitempty"`
	Metadata     map[string]string `yaml:"metadata,omitempty"`
	Timestamps

	// Content holds the markdown body (not stored in frontmatter)
	Content string `yaml:"-"`
}

// NewTask creates a new Task with generated ID and timestamps
func NewTask(title, projectID, areaID string) *Task {
	t := &Task{
		ID:           fmt.Sprintf("task-%s", uuid.New().String()[:8]),
		Title:        title,
		Type:         "task",
		ProjectID:    projectID,
		AreaID:       areaID,
		Status:       TaskStatusPending,
		Priority:     PriorityMedium,
		Tags:         []string{},
		Dependencies: []string{},
		Metadata:     make(map[string]string),
	}
	t.SetCreated()
	return t
}

// Slug returns a URL-safe identifier derived from the title
func (t *Task) Slug() string {
	slug := strings.ToLower(t.Title)
	slug = strings.ReplaceAll(slug, " ", "-")
	var result strings.Builder
	for _, r := range slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// Validate checks if the task has all required fields
func (t *Task) Validate() error {
	if t.ID == "" {
		return fmt.Errorf("task ID is required")
	}
	if t.Title == "" {
		return fmt.Errorf("task title is required")
	}
	if t.Type != "task" {
		return fmt.Errorf("task type must be 'task', got '%s'", t.Type)
	}
	if t.ProjectID == "" {
		return fmt.Errorf("task project_id is required")
	}
	if t.AreaID == "" {
		return fmt.Errorf("task area_id is required")
	}
	return nil
}

// IsComplete returns true if the task is completed
func (t *Task) IsComplete() bool {
	return t.Status == TaskStatusCompleted
}

// IsPending returns true if the task hasn't been started
func (t *Task) IsPending() bool {
	return t.Status == TaskStatusPending
}

// IsBlocked returns true if the task is blocked
func (t *Task) IsBlocked() bool {
	return t.Status == TaskStatusBlocked
}

// Complete marks the task as completed
func (t *Task) Complete() {
	t.Status = TaskStatusCompleted
	t.UpdateTimestamp()
}

// Start marks the task as in progress
func (t *Task) Start() {
	t.Status = TaskStatusInProgress
	t.UpdateTimestamp()
}

// Block marks the task as blocked
func (t *Task) Block() {
	t.Status = TaskStatusBlocked
	t.UpdateTimestamp()
}

// Cancel marks the task as cancelled
func (t *Task) Cancel() {
	t.Status = TaskStatusCancelled
	t.UpdateTimestamp()
}

// Reopen sets the task back to pending
func (t *Task) Reopen() {
	t.Status = TaskStatusPending
	t.UpdateTimestamp()
}

// AddTag adds a tag if it doesn't already exist
func (t *Task) AddTag(tag string) {
	tag = strings.ToLower(strings.TrimSpace(tag))
	for _, existing := range t.Tags {
		if existing == tag {
			return
		}
	}
	t.Tags = append(t.Tags, tag)
	t.UpdateTimestamp()
}

// RemoveTag removes a tag if it exists
func (t *Task) RemoveTag(tag string) {
	tag = strings.ToLower(strings.TrimSpace(tag))
	for i, existing := range t.Tags {
		if existing == tag {
			t.Tags = append(t.Tags[:i], t.Tags[i+1:]...)
			t.UpdateTimestamp()
			return
		}
	}
}

// HasTag returns true if the task has the specified tag
func (t *Task) HasTag(tag string) bool {
	tag = strings.ToLower(strings.TrimSpace(tag))
	for _, existing := range t.Tags {
		if existing == tag {
			return true
		}
	}
	return false
}

// AddDependency adds a task ID as a dependency
func (t *Task) AddDependency(taskID string) {
	for _, dep := range t.Dependencies {
		if dep == taskID {
			return
		}
	}
	t.Dependencies = append(t.Dependencies, taskID)
	t.UpdateTimestamp()
}

// RemoveDependency removes a dependency if it exists
func (t *Task) RemoveDependency(taskID string) {
	for i, dep := range t.Dependencies {
		if dep == taskID {
			t.Dependencies = append(t.Dependencies[:i], t.Dependencies[i+1:]...)
			t.UpdateTimestamp()
			return
		}
	}
}

// HasDependency returns true if the task depends on the specified task
func (t *Task) HasDependency(taskID string) bool {
	for _, dep := range t.Dependencies {
		if dep == taskID {
			return true
		}
	}
	return false
}

// IsOverdue returns true if the task has a due date that has passed
func (t *Task) IsOverdue() bool {
	if t.DueDate == nil || t.IsComplete() {
		return false
	}
	return time.Now().After(*t.DueDate)
}

// DaysUntilDue returns the number of days until the due date
// Returns -1 if there's no due date
func (t *Task) DaysUntilDue() int {
	if t.DueDate == nil {
		return -1
	}
	duration := time.Until(*t.DueDate)
	return int(duration.Hours() / 24)
}
