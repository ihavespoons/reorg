package service

import (
	"context"

	"github.com/ihavespoons/reorg/internal/domain"
	"github.com/ihavespoons/reorg/internal/storage/markdown"
)

// LocalClient implements ReorgClient by embedding services directly.
// This is used in embedded mode where no network calls are needed.
type LocalClient struct {
	store *markdown.Store
}

// NewLocalClient creates a new local client with direct access to storage
func NewLocalClient(store *markdown.Store) *LocalClient {
	return &LocalClient{store: store}
}

// Store returns the underlying store for direct access when needed
func (c *LocalClient) Store() *markdown.Store {
	return c.store
}

// AreaService implementation

func (c *LocalClient) CreateArea(ctx context.Context, area *domain.Area) (*domain.Area, error) {
	if err := c.store.Areas().Create(ctx, area); err != nil {
		return nil, err
	}
	return area, nil
}

func (c *LocalClient) GetArea(ctx context.Context, id string) (*domain.Area, error) {
	return c.store.Areas().Get(ctx, id)
}

func (c *LocalClient) GetAreaBySlug(ctx context.Context, slug string) (*domain.Area, error) {
	return c.store.Areas().GetBySlug(ctx, slug)
}

func (c *LocalClient) ListAreas(ctx context.Context) ([]*domain.Area, error) {
	return c.store.Areas().List(ctx)
}

func (c *LocalClient) UpdateArea(ctx context.Context, area *domain.Area) error {
	return c.store.Areas().Update(ctx, area)
}

func (c *LocalClient) DeleteArea(ctx context.Context, id string) error {
	return c.store.Areas().Delete(ctx, id)
}

// ProjectService implementation

func (c *LocalClient) CreateProject(ctx context.Context, project *domain.Project) (*domain.Project, error) {
	if err := c.store.Projects().Create(ctx, project); err != nil {
		return nil, err
	}
	return project, nil
}

func (c *LocalClient) GetProject(ctx context.Context, id string) (*domain.Project, error) {
	return c.store.Projects().Get(ctx, id)
}

func (c *LocalClient) GetProjectBySlug(ctx context.Context, areaID, slug string) (*domain.Project, error) {
	// First get the area to find its slug
	area, err := c.store.Areas().Get(ctx, areaID)
	if err != nil {
		return nil, err
	}
	return c.store.Projects().GetBySlug(ctx, area.Slug(), slug)
}

func (c *LocalClient) ListProjects(ctx context.Context, areaID string) ([]*domain.Project, error) {
	return c.store.Projects().List(ctx, areaID)
}

func (c *LocalClient) ListAllProjects(ctx context.Context) ([]*domain.Project, error) {
	return c.store.Projects().ListAll(ctx)
}

func (c *LocalClient) UpdateProject(ctx context.Context, project *domain.Project) error {
	return c.store.Projects().Update(ctx, project)
}

func (c *LocalClient) DeleteProject(ctx context.Context, id string) error {
	return c.store.Projects().Delete(ctx, id)
}

func (c *LocalClient) CompleteProject(ctx context.Context, id string) error {
	project, err := c.store.Projects().Get(ctx, id)
	if err != nil {
		return err
	}
	project.Complete()
	return c.store.Projects().Update(ctx, project)
}

// TaskService implementation

func (c *LocalClient) CreateTask(ctx context.Context, task *domain.Task) (*domain.Task, error) {
	if err := c.store.Tasks().Create(ctx, task); err != nil {
		return nil, err
	}
	return task, nil
}

func (c *LocalClient) GetTask(ctx context.Context, id string) (*domain.Task, error) {
	return c.store.Tasks().Get(ctx, id)
}

func (c *LocalClient) GetTaskBySlug(ctx context.Context, projectID, slug string) (*domain.Task, error) {
	// Get the project and area to find slugs
	project, err := c.store.Projects().Get(ctx, projectID)
	if err != nil {
		return nil, err
	}
	area, err := c.store.Areas().Get(ctx, project.AreaID)
	if err != nil {
		return nil, err
	}
	return c.store.Tasks().GetBySlug(ctx, area.Slug(), project.Slug(), slug)
}

func (c *LocalClient) ListTasks(ctx context.Context, projectID string) ([]*domain.Task, error) {
	return c.store.Tasks().List(ctx, projectID)
}

func (c *LocalClient) ListTasksByArea(ctx context.Context, areaID string) ([]*domain.Task, error) {
	return c.store.Tasks().ListByArea(ctx, areaID)
}

func (c *LocalClient) ListAllTasks(ctx context.Context) ([]*domain.Task, error) {
	return c.store.Tasks().ListAll(ctx)
}

func (c *LocalClient) UpdateTask(ctx context.Context, task *domain.Task) error {
	return c.store.Tasks().Update(ctx, task)
}

func (c *LocalClient) DeleteTask(ctx context.Context, id string) error {
	return c.store.Tasks().Delete(ctx, id)
}

func (c *LocalClient) StartTask(ctx context.Context, id string) error {
	task, err := c.store.Tasks().Get(ctx, id)
	if err != nil {
		return err
	}
	task.Start()
	return c.store.Tasks().Update(ctx, task)
}

func (c *LocalClient) CompleteTask(ctx context.Context, id string) error {
	task, err := c.store.Tasks().Get(ctx, id)
	if err != nil {
		return err
	}
	task.Complete()
	return c.store.Tasks().Update(ctx, task)
}

// Ensure LocalClient implements ReorgClient
var _ ReorgClient = (*LocalClient)(nil)
