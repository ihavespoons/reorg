package service

import (
	"context"

	"github.com/ihavespoons/reorg/internal/domain"
)

// ReorgClient is the key abstraction enabling embedded/remote modes.
// Both LocalClient and RemoteClient implement this interface.
type ReorgClient interface {
	AreaService
	ProjectService
	TaskService
}

// AreaService defines area operations
type AreaService interface {
	CreateArea(ctx context.Context, area *domain.Area) (*domain.Area, error)
	GetArea(ctx context.Context, id string) (*domain.Area, error)
	GetAreaBySlug(ctx context.Context, slug string) (*domain.Area, error)
	ListAreas(ctx context.Context) ([]*domain.Area, error)
	UpdateArea(ctx context.Context, area *domain.Area) error
	DeleteArea(ctx context.Context, id string) error
}

// ProjectService defines project operations
type ProjectService interface {
	CreateProject(ctx context.Context, project *domain.Project) (*domain.Project, error)
	GetProject(ctx context.Context, id string) (*domain.Project, error)
	GetProjectBySlug(ctx context.Context, areaID, slug string) (*domain.Project, error)
	ListProjects(ctx context.Context, areaID string) ([]*domain.Project, error)
	ListAllProjects(ctx context.Context) ([]*domain.Project, error)
	UpdateProject(ctx context.Context, project *domain.Project) error
	DeleteProject(ctx context.Context, id string) error
	CompleteProject(ctx context.Context, id string) error
}

// TaskService defines task operations
type TaskService interface {
	CreateTask(ctx context.Context, task *domain.Task) (*domain.Task, error)
	GetTask(ctx context.Context, id string) (*domain.Task, error)
	GetTaskBySlug(ctx context.Context, projectID, slug string) (*domain.Task, error)
	ListTasks(ctx context.Context, projectID string) ([]*domain.Task, error)
	ListTasksByArea(ctx context.Context, areaID string) ([]*domain.Task, error)
	ListAllTasks(ctx context.Context) ([]*domain.Task, error)
	UpdateTask(ctx context.Context, task *domain.Task) error
	DeleteTask(ctx context.Context, id string) error
	StartTask(ctx context.Context, id string) error
	CompleteTask(ctx context.Context, id string) error
}
