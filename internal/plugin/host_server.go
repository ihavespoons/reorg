package plugin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/ihavespoons/reorg/internal/domain"
	"github.com/ihavespoons/reorg/internal/llm"
	"github.com/ihavespoons/reorg/internal/service"
	"github.com/ihavespoons/reorg/pkg/plugin"
)

// HostClient implements plugin.HostClient by wrapping the service client.
type HostClient struct {
	client    service.ReorgClient
	llmClient llm.Client
	stateDir  string
}

// NewHostClient creates a new host client for plugins.
func NewHostClient(client service.ReorgClient, llmClient llm.Client, stateDir string) *HostClient {
	return &HostClient{
		client:    client,
		llmClient: llmClient,
		stateDir:  stateDir,
	}
}

// ListAreas returns all areas.
func (h *HostClient) ListAreas(ctx context.Context) ([]*plugin.Area, error) {
	areas, err := h.client.ListAreas(ctx)
	if err != nil {
		return nil, err
	}

	var result []*plugin.Area
	for _, a := range areas {
		result = append(result, domainAreaToPlugin(a))
	}
	return result, nil
}

// GetArea returns an area by ID.
func (h *HostClient) GetArea(ctx context.Context, id string) (*plugin.Area, error) {
	area, err := h.client.GetArea(ctx, id)
	if err != nil {
		// Try by slug
		area, err = h.client.GetAreaBySlug(ctx, id)
		if err != nil {
			return nil, err
		}
	}
	return domainAreaToPlugin(area), nil
}

// CreateArea creates a new area.
func (h *HostClient) CreateArea(ctx context.Context, title, content string, tags []string) (*plugin.Area, error) {
	area := domain.NewArea(title)
	area.Content = content
	// Note: domain.Area doesn't have Tags, they're stored in Metadata if needed

	created, err := h.client.CreateArea(ctx, area)
	if err != nil {
		return nil, err
	}
	return domainAreaToPlugin(created), nil
}

// ListProjects returns projects for an area.
func (h *HostClient) ListProjects(ctx context.Context, areaID string) ([]*plugin.Project, error) {
	projects, err := h.client.ListProjects(ctx, areaID)
	if err != nil {
		return nil, err
	}

	var result []*plugin.Project
	for _, p := range projects {
		result = append(result, domainProjectToPlugin(p))
	}
	return result, nil
}

// ListAllProjects returns all projects.
func (h *HostClient) ListAllProjects(ctx context.Context) ([]*plugin.Project, error) {
	projects, err := h.client.ListAllProjects(ctx)
	if err != nil {
		return nil, err
	}

	var result []*plugin.Project
	for _, p := range projects {
		result = append(result, domainProjectToPlugin(p))
	}
	return result, nil
}

// GetProject returns a project by ID.
func (h *HostClient) GetProject(ctx context.Context, id string) (*plugin.Project, error) {
	project, err := h.client.GetProject(ctx, id)
	if err != nil {
		return nil, err
	}
	return domainProjectToPlugin(project), nil
}

// CreateProject creates a new project.
func (h *HostClient) CreateProject(ctx context.Context, title, areaID, content string, tags []string) (*plugin.Project, error) {
	project := domain.NewProject(title, areaID)
	project.Content = content
	for _, tag := range tags {
		project.AddTag(tag)
	}

	created, err := h.client.CreateProject(ctx, project)
	if err != nil {
		return nil, err
	}
	return domainProjectToPlugin(created), nil
}

// ListTasks returns tasks for a project.
func (h *HostClient) ListTasks(ctx context.Context, projectID string) ([]*plugin.Task, error) {
	tasks, err := h.client.ListTasks(ctx, projectID)
	if err != nil {
		return nil, err
	}

	var result []*plugin.Task
	for _, t := range tasks {
		result = append(result, domainTaskToPlugin(t))
	}
	return result, nil
}

// CreateTask creates a new task.
func (h *HostClient) CreateTask(ctx context.Context, title, projectID, areaID, content string, priority plugin.Priority, tags []string) (*plugin.Task, error) {
	task := domain.NewTask(title, projectID, areaID)
	task.Content = content
	task.Priority = pluginPriorityToDomain(priority)
	for _, tag := range tags {
		task.AddTag(tag)
	}

	created, err := h.client.CreateTask(ctx, task)
	if err != nil {
		return nil, err
	}
	return domainTaskToPlugin(created), nil
}

// CategorizeWithContext categorizes content using AI.
func (h *HostClient) CategorizeWithContext(ctx context.Context, content string, existingProjects []plugin.ProjectContext) (*plugin.CategorizeResult, error) {
	if h.llmClient == nil {
		return nil, nil
	}

	var llmProjects []llm.ProjectContext
	for _, p := range existingProjects {
		llmProjects = append(llmProjects, llm.ProjectContext{
			ID:    p.ID,
			Title: p.Title,
			Area:  p.Area,
		})
	}

	result, err := h.llmClient.CategorizeWithContext(ctx, content, llmProjects)
	if err != nil {
		return nil, err
	}

	return &plugin.CategorizeResult{
		Area:              result.Area,
		AreaConfidence:    result.AreaConfidence,
		ProjectID:         result.ProjectID,
		ProjectSuggestion: result.ProjectSuggestion,
		Tags:              result.Tags,
		Summary:           result.Summary,
		IsActionable:      result.IsActionable,
	}, nil
}

// ExtractTasks extracts tasks from content using AI.
func (h *HostClient) ExtractTasks(ctx context.Context, content string) ([]plugin.ExtractedTask, error) {
	if h.llmClient == nil {
		return nil, nil
	}

	tasks, err := h.llmClient.ExtractTasks(ctx, content)
	if err != nil {
		return nil, err
	}

	var result []plugin.ExtractedTask
	for _, t := range tasks {
		result = append(result, plugin.ExtractedTask{
			Title:       t.Title,
			Description: t.Description,
			Priority:    t.Priority,
			DueDate:     t.DueDate,
			Tags:        t.Tags,
		})
	}
	return result, nil
}

// GetState retrieves a state value.
func (h *HostClient) GetState(ctx context.Context, key string) ([]byte, bool, error) {
	if err := os.MkdirAll(h.stateDir, 0755); err != nil {
		return nil, false, err
	}

	path := filepath.Join(h.stateDir, sanitizeKey(key)+".json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

// SetState stores a state value.
func (h *HostClient) SetState(ctx context.Context, key string, value []byte) error {
	if err := os.MkdirAll(h.stateDir, 0755); err != nil {
		return err
	}

	path := filepath.Join(h.stateDir, sanitizeKey(key)+".json")
	return os.WriteFile(path, value, 0644)
}

// DeleteState removes a state value.
func (h *HostClient) DeleteState(ctx context.Context, key string) error {
	path := filepath.Join(h.stateDir, sanitizeKey(key)+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// FindOrCreateArea finds an area by name/slug or creates it.
func (h *HostClient) FindOrCreateArea(ctx context.Context, name string) (*plugin.Area, error) {
	areas, err := h.client.ListAreas(ctx)
	if err != nil {
		return nil, err
	}

	slug := slugify(name)
	for _, a := range areas {
		if strings.EqualFold(a.Slug(), slug) || strings.EqualFold(a.Title, name) {
			return domainAreaToPlugin(a), nil
		}
	}

	// Create new area
	titleCaser := cases.Title(language.English)
	area := domain.NewArea(titleCaser.String(name))
	created, err := h.client.CreateArea(ctx, area)
	if err != nil {
		return nil, err
	}
	return domainAreaToPlugin(created), nil
}

// FindOrCreateProject finds a project by name/slug or creates it.
func (h *HostClient) FindOrCreateProject(ctx context.Context, name, areaID, content string, tags []string) (*plugin.Project, error) {
	projects, err := h.client.ListProjects(ctx, areaID)
	if err != nil {
		return nil, err
	}

	slug := slugify(name)
	for _, p := range projects {
		if strings.EqualFold(p.Slug(), slug) || strings.EqualFold(p.Title, name) {
			return domainProjectToPlugin(p), nil
		}
	}

	// Create new project
	project := domain.NewProject(name, areaID)
	project.Content = content
	for _, tag := range tags {
		project.AddTag(tag)
	}

	created, err := h.client.CreateProject(ctx, project)
	if err != nil {
		return nil, err
	}
	return domainProjectToPlugin(created), nil
}

// BuildProjectContext builds a list of existing projects for AI matching.
func (h *HostClient) BuildProjectContext(ctx context.Context) ([]plugin.ProjectContext, error) {
	var result []plugin.ProjectContext

	areas, err := h.client.ListAreas(ctx)
	if err != nil {
		return result, err
	}

	for _, area := range areas {
		projects, err := h.client.ListProjects(ctx, area.ID)
		if err != nil {
			continue
		}
		for _, p := range projects {
			result = append(result, plugin.ProjectContext{
				ID:    p.ID,
				Title: p.Title,
				Area:  area.Title,
			})
		}
	}

	return result, nil
}

// PluginState provides JSON-based state storage helpers.
type PluginState struct {
	host *HostClient
}

// NewPluginState creates a new plugin state helper.
func NewPluginState(host *HostClient) *PluginState {
	return &PluginState{host: host}
}

// Load loads state from a key.
func (s *PluginState) Load(ctx context.Context, key string, v interface{}) error {
	data, found, err := s.host.GetState(ctx, key)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	return json.Unmarshal(data, v)
}

// Save saves state to a key.
func (s *PluginState) Save(ctx context.Context, key string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return s.host.SetState(ctx, key, data)
}

// Helper functions

func sanitizeKey(key string) string {
	// Replace potentially problematic characters
	key = strings.ReplaceAll(key, "/", "_")
	key = strings.ReplaceAll(key, "\\", "_")
	key = strings.ReplaceAll(key, "..", "_")
	return key
}

func slugify(s string) string {
	slug := strings.ToLower(s)
	slug = strings.ReplaceAll(slug, " ", "-")
	var result strings.Builder
	for _, r := range slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		}
	}
	return result.String()
}

func domainAreaToPlugin(a *domain.Area) *plugin.Area {
	if a == nil {
		return nil
	}
	return &plugin.Area{
		ID:        a.ID,
		Title:     a.Title,
		Content:   a.Content,
		Tags:      nil, // domain.Area doesn't have Tags
		CreatedAt: a.Created,
		UpdatedAt: a.Updated,
	}
}

func domainProjectToPlugin(p *domain.Project) *plugin.Project {
	if p == nil {
		return nil
	}
	proj := &plugin.Project{
		ID:        p.ID,
		Title:     p.Title,
		AreaID:    p.AreaID,
		Content:   p.Content,
		Status:    plugin.ProjectStatus(p.Status),
		Tags:      p.Tags,
		CreatedAt: p.Created,
		UpdatedAt: p.Updated,
	}
	if p.DueDate != nil {
		proj.DueDate = p.DueDate
	}
	return proj
}

func domainTaskToPlugin(t *domain.Task) *plugin.Task {
	if t == nil {
		return nil
	}
	task := &plugin.Task{
		ID:        t.ID,
		Title:     t.Title,
		ProjectID: t.ProjectID,
		AreaID:    t.AreaID,
		Content:   t.Content,
		Status:    domainTaskStatusToPlugin(t.Status),
		Priority:  domainPriorityToPlugin(t.Priority),
		Tags:      t.Tags,
		CreatedAt: t.Created,
		UpdatedAt: t.Updated,
	}
	if t.DueDate != nil {
		task.DueDate = t.DueDate
	}
	return task
}

func domainTaskStatusToPlugin(s domain.TaskStatus) plugin.TaskStatus {
	switch s {
	case domain.TaskStatusPending:
		return plugin.TaskStatusPending
	case domain.TaskStatusInProgress:
		return plugin.TaskStatusInProgress
	case domain.TaskStatusCompleted:
		return plugin.TaskStatusCompleted
	case domain.TaskStatusBlocked:
		return plugin.TaskStatusBlocked
	case domain.TaskStatusCancelled:
		return plugin.TaskStatusCancelled
	default:
		return plugin.TaskStatusPending
	}
}

func domainPriorityToPlugin(p domain.Priority) plugin.Priority {
	switch p {
	case domain.PriorityLow:
		return plugin.PriorityLow
	case domain.PriorityMedium:
		return plugin.PriorityMedium
	case domain.PriorityHigh:
		return plugin.PriorityHigh
	case domain.PriorityUrgent:
		return plugin.PriorityUrgent
	default:
		return plugin.PriorityMedium
	}
}

func pluginPriorityToDomain(p plugin.Priority) domain.Priority {
	switch p {
	case plugin.PriorityLow:
		return domain.PriorityLow
	case plugin.PriorityMedium:
		return domain.PriorityMedium
	case plugin.PriorityHigh:
		return domain.PriorityHigh
	case plugin.PriorityUrgent:
		return domain.PriorityUrgent
	default:
		return domain.PriorityMedium
	}
}

// Ensure interface is satisfied
var _ plugin.HostClient = (*HostClient)(nil)
