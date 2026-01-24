// Package plugin provides the SDK for building reorg plugins.
//
// Plugins are separate executables that communicate with the reorg host
// via gRPC using hashicorp/go-plugin. This package provides:
//
//   - Plugin interface: what plugins must implement
//   - HostClient interface: what plugins can call on the host
//   - Serve() function: to start a plugin server
//
// Example plugin implementation:
//
//	package main
//
//	import "github.com/ihavespoons/reorg/pkg/plugin"
//
//	type MyPlugin struct{}
//
//	func (p *MyPlugin) GetManifest(ctx context.Context) (*plugin.Manifest, error) {
//	    return &plugin.Manifest{
//	        Name:     "my-plugin",
//	        Version:  "1.0.0",
//	        Schedule: "*/15 * * * *",
//	    }, nil
//	}
//
//	// ... implement other methods ...
//
//	func main() {
//	    plugin.Serve(&MyPlugin{})
//	}
package plugin

import (
	"context"
	"time"
)

// Plugin is the interface that all reorg plugins must implement.
type Plugin interface {
	// GetManifest returns the plugin's metadata and capabilities.
	GetManifest(ctx context.Context) (*Manifest, error)

	// Configure sets up the plugin with host-provided configuration.
	// The host parameter provides access to reorg data and LLM services.
	Configure(ctx context.Context, host HostClient, config map[string]string, stateDir string) error

	// Execute runs the plugin's main logic.
	Execute(ctx context.Context, params *ExecuteParams) (*ExecuteResult, error)

	// Shutdown gracefully stops the plugin.
	Shutdown(ctx context.Context) error
}

// HostClient provides access to reorg data and services from within a plugin.
type HostClient interface {
	// Area operations
	ListAreas(ctx context.Context) ([]*Area, error)
	GetArea(ctx context.Context, id string) (*Area, error)
	CreateArea(ctx context.Context, title, content string, tags []string) (*Area, error)
	FindOrCreateArea(ctx context.Context, name string) (*Area, error)

	// Project operations
	ListProjects(ctx context.Context, areaID string) ([]*Project, error)
	ListAllProjects(ctx context.Context) ([]*Project, error)
	GetProject(ctx context.Context, id string) (*Project, error)
	CreateProject(ctx context.Context, title, areaID, content string, tags []string) (*Project, error)
	FindOrCreateProject(ctx context.Context, name, areaID, content string, tags []string) (*Project, error)

	// Task operations
	ListTasks(ctx context.Context, projectID string) ([]*Task, error)
	CreateTask(ctx context.Context, title, projectID, areaID, content string, priority Priority, tags []string) (*Task, error)

	// Helper methods
	BuildProjectContext(ctx context.Context) ([]ProjectContext, error)

	// LLM operations (proxied through host for centralized API key management)
	CategorizeWithContext(ctx context.Context, content string, existingProjects []ProjectContext) (*CategorizeResult, error)
	ExtractTasks(ctx context.Context, content string) ([]ExtractedTask, error)

	// State persistence (per-plugin key-value storage)
	GetState(ctx context.Context, key string) ([]byte, bool, error)
	SetState(ctx context.Context, key string, value []byte) error
	DeleteState(ctx context.Context, key string) error
}

// Manifest describes a plugin's metadata and capabilities.
type Manifest struct {
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Description  string   `json:"description"`
	Author       string   `json:"author"`
	Schedule     string   `json:"schedule"`      // Cron expression (e.g., "*/15 * * * *")
	Capabilities []string `json:"capabilities"`  // e.g., ["import", "sync"]
	ConfigSchema string   `json:"config_schema"` // JSON Schema for configuration
}

// ExecuteParams contains parameters passed to plugin execution.
type ExecuteParams struct {
	DryRun bool              `json:"dry_run"`
	Params map[string]string `json:"params"`
}

// ExecuteResult contains the results of plugin execution.
type ExecuteResult struct {
	Success bool             `json:"success"`
	Error   string           `json:"error,omitempty"`
	Summary *ExecuteSummary  `json:"summary,omitempty"`
	Results []ExecuteItem    `json:"results,omitempty"`
}

// ExecuteSummary provides a high-level overview of execution.
type ExecuteSummary struct {
	ItemsProcessed int    `json:"items_processed"`
	ItemsImported  int    `json:"items_imported"`
	ItemsSkipped   int    `json:"items_skipped"`
	ItemsFailed    int    `json:"items_failed"`
	Message        string `json:"message,omitempty"`
}

// ExecuteItem represents a single item processed during execution.
type ExecuteItem struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Action   string            `json:"action"` // "imported", "skipped", "failed"
	Message  string            `json:"message,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Area represents a top-level organizational category.
type Area struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Tags      []string  `json:"tags"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Project represents a project within an area.
type Project struct {
	ID        string        `json:"id"`
	Title     string        `json:"title"`
	AreaID    string        `json:"area_id"`
	Content   string        `json:"content"`
	Status    ProjectStatus `json:"status"`
	Tags      []string      `json:"tags"`
	DueDate   *time.Time    `json:"due_date,omitempty"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

// ProjectStatus represents the state of a project.
type ProjectStatus string

const (
	ProjectStatusActive    ProjectStatus = "active"
	ProjectStatusOnHold    ProjectStatus = "on_hold"
	ProjectStatusCompleted ProjectStatus = "completed"
	ProjectStatusArchived  ProjectStatus = "archived"
)

// Task represents a task within a project.
type Task struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	ProjectID string     `json:"project_id"`
	AreaID    string     `json:"area_id"`
	Content   string     `json:"content"`
	Status    TaskStatus `json:"status"`
	Priority  Priority   `json:"priority"`
	Tags      []string   `json:"tags"`
	DueDate   *time.Time `json:"due_date,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// TaskStatus represents the state of a task.
type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusCompleted  TaskStatus = "completed"
	TaskStatusBlocked    TaskStatus = "blocked"
	TaskStatusCancelled  TaskStatus = "cancelled"
)

// Priority represents the urgency level.
type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityMedium Priority = "medium"
	PriorityHigh   Priority = "high"
	PriorityUrgent Priority = "urgent"
)

// ProjectContext provides context about existing projects for AI matching.
type ProjectContext struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Area  string `json:"area"`
}

// CategorizeResult contains the AI categorization of content.
type CategorizeResult struct {
	Area              string   `json:"area"`
	AreaConfidence    float64  `json:"area_confidence"`
	ProjectID         string   `json:"project_id,omitempty"`         // ID of existing project if matched
	ProjectSuggestion string   `json:"project_suggestion,omitempty"` // Suggested name for new project
	Tags              []string `json:"tags,omitempty"`
	Summary           string   `json:"summary"`
	IsActionable      bool     `json:"is_actionable"`
}

// ExtractedTask represents a task extracted from content by AI.
type ExtractedTask struct {
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Priority    string   `json:"priority,omitempty"`
	DueDate     string   `json:"due_date,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}
