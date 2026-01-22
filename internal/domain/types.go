package domain

import "time"

// Priority represents the urgency level of a project or task
type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityMedium Priority = "medium"
	PriorityHigh   Priority = "high"
	PriorityUrgent Priority = "urgent"
)

// ProjectStatus represents the current state of a project
type ProjectStatus string

const (
	ProjectStatusActive    ProjectStatus = "active"
	ProjectStatusOnHold    ProjectStatus = "on_hold"
	ProjectStatusCompleted ProjectStatus = "completed"
	ProjectStatusArchived  ProjectStatus = "archived"
)

// TaskStatus represents the current state of a task
type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusCompleted  TaskStatus = "completed"
	TaskStatusBlocked    TaskStatus = "blocked"
	TaskStatusCancelled  TaskStatus = "cancelled"
)

// Timestamps holds common timestamp fields
type Timestamps struct {
	Created time.Time `yaml:"created"`
	Updated time.Time `yaml:"updated"`
}

// UpdateTimestamp sets the Updated field to now
func (t *Timestamps) UpdateTimestamp() {
	t.Updated = time.Now().UTC()
}

// SetCreated sets both Created and Updated to now
func (t *Timestamps) SetCreated() {
	now := time.Now().UTC()
	t.Created = now
	t.Updated = now
}
