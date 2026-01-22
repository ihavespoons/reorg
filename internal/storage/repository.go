package storage

import (
	"context"

	"github.com/ihavespoons/reorg/internal/domain"
)

// AreaRepository defines operations for storing and retrieving Areas
type AreaRepository interface {
	// Create stores a new area
	Create(ctx context.Context, area *domain.Area) error

	// Get retrieves an area by ID
	Get(ctx context.Context, id string) (*domain.Area, error)

	// GetBySlug retrieves an area by its slug
	GetBySlug(ctx context.Context, slug string) (*domain.Area, error)

	// List returns all areas
	List(ctx context.Context) ([]*domain.Area, error)

	// Update saves changes to an existing area
	Update(ctx context.Context, area *domain.Area) error

	// Delete removes an area by ID
	Delete(ctx context.Context, id string) error
}

// ProjectRepository defines operations for storing and retrieving Projects
type ProjectRepository interface {
	// Create stores a new project
	Create(ctx context.Context, project *domain.Project) error

	// Get retrieves a project by ID
	Get(ctx context.Context, id string) (*domain.Project, error)

	// GetBySlug retrieves a project by its slug within an area
	GetBySlug(ctx context.Context, areaSlug, projectSlug string) (*domain.Project, error)

	// List returns all projects, optionally filtered by area
	List(ctx context.Context, areaID string) ([]*domain.Project, error)

	// ListAll returns all projects across all areas
	ListAll(ctx context.Context) ([]*domain.Project, error)

	// Update saves changes to an existing project
	Update(ctx context.Context, project *domain.Project) error

	// Delete removes a project by ID
	Delete(ctx context.Context, id string) error
}

// TaskRepository defines operations for storing and retrieving Tasks
type TaskRepository interface {
	// Create stores a new task
	Create(ctx context.Context, task *domain.Task) error

	// Get retrieves a task by ID
	Get(ctx context.Context, id string) (*domain.Task, error)

	// GetBySlug retrieves a task by its slug within a project
	GetBySlug(ctx context.Context, areaSlug, projectSlug, taskSlug string) (*domain.Task, error)

	// List returns all tasks for a project
	List(ctx context.Context, projectID string) ([]*domain.Task, error)

	// ListByArea returns all tasks for an area
	ListByArea(ctx context.Context, areaID string) ([]*domain.Task, error)

	// ListAll returns all tasks
	ListAll(ctx context.Context) ([]*domain.Task, error)

	// Update saves changes to an existing task
	Update(ctx context.Context, task *domain.Task) error

	// Delete removes a task by ID
	Delete(ctx context.Context, id string) error
}

// TaskFilter defines filtering options for listing tasks
type TaskFilter struct {
	Status   *domain.TaskStatus
	Priority *domain.Priority
	Tags     []string
	Overdue  *bool
}
