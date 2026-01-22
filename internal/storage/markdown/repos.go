package markdown

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ihavespoons/reorg/internal/domain"
	"github.com/ihavespoons/reorg/internal/storage"
	"github.com/ihavespoons/reorg/internal/storage/git"
)

// Store provides file-based storage for all domain objects
type Store struct {
	rootDir    string
	parser     *Parser
	writer     *Writer
	git        *git.Client
	autoCommit bool
}

// NewStore creates a new file-based store
func NewStore(rootDir string) *Store {
	gitClient, _ := git.NewClient(rootDir)
	return &Store{
		rootDir:    rootDir,
		parser:     NewParser(),
		writer:     NewWriter(),
		git:        gitClient,
		autoCommit: true, // Enable by default
	}
}

// SetAutoCommit enables or disables automatic git commits
func (s *Store) SetAutoCommit(enabled bool) {
	s.autoCommit = enabled
}

// Git returns the git client
func (s *Store) Git() *git.Client {
	return s.git
}

// commit performs an auto-commit if enabled
func (s *Store) commit(action string) {
	if s.autoCommit && s.git != nil {
		_ = s.git.AutoCommit(action)
	}
}

// RootDir returns the root directory of the store
func (s *Store) RootDir() string {
	return s.rootDir
}

// Initialize creates the directory structure
func (s *Store) Initialize() error {
	dirs := []string{
		s.rootDir,
		filepath.Join(s.rootDir, "areas"),
		filepath.Join(s.rootDir, "inbox"),
		filepath.Join(s.rootDir, "archive"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// AreaRepository implementation

// AreaRepo implements storage.AreaRepository
type AreaRepo struct {
	store *Store
}

// NewAreaRepo creates a new AreaRepo
func (s *Store) Areas() *AreaRepo {
	return &AreaRepo{store: s}
}

func (r *AreaRepo) areaDir(slug string) string {
	return filepath.Join(r.store.rootDir, "areas", slug)
}

func (r *AreaRepo) areaFile(slug string) string {
	return filepath.Join(r.areaDir(slug), slug+".md")
}

// Create stores a new area
func (r *AreaRepo) Create(ctx context.Context, area *domain.Area) error {
	if err := area.Validate(); err != nil {
		return err
	}

	slug := area.Slug()
	areaDir := r.areaDir(slug)

	// Check if area already exists
	if _, err := os.Stat(areaDir); err == nil {
		return fmt.Errorf("area '%s' already exists", slug)
	}

	// Create area directory structure
	projectsDir := filepath.Join(areaDir, "projects")
	if err := os.MkdirAll(projectsDir, 0755); err != nil {
		return fmt.Errorf("failed to create area directory: %w", err)
	}

	// Write area file
	if err := r.store.writer.WriteAreaToFile(r.areaFile(slug), area); err != nil {
		// Clean up on failure
		_ = os.RemoveAll(areaDir)
		return err
	}

	r.store.commit(fmt.Sprintf("create area: %s", area.Title))
	return nil
}

// Get retrieves an area by ID
func (r *AreaRepo) Get(ctx context.Context, id string) (*domain.Area, error) {
	areas, err := r.List(ctx)
	if err != nil {
		return nil, err
	}

	for _, area := range areas {
		if area.ID == id {
			return area, nil
		}
	}

	return nil, fmt.Errorf("area not found: %s", id)
}

// GetBySlug retrieves an area by its slug
func (r *AreaRepo) GetBySlug(ctx context.Context, slug string) (*domain.Area, error) {
	areaFile := r.areaFile(slug)
	if _, err := os.Stat(areaFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("area not found: %s", slug)
	}

	return r.store.parser.ParseAreaFromFile(areaFile)
}

// List returns all areas
func (r *AreaRepo) List(ctx context.Context) ([]*domain.Area, error) {
	areasDir := filepath.Join(r.store.rootDir, "areas")
	entries, err := os.ReadDir(areasDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*domain.Area{}, nil
		}
		return nil, fmt.Errorf("failed to read areas directory: %w", err)
	}

	var areas []*domain.Area
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		slug := entry.Name()
		areaFile := r.areaFile(slug)

		if _, err := os.Stat(areaFile); os.IsNotExist(err) {
			continue // Skip directories without area file
		}

		area, err := r.store.parser.ParseAreaFromFile(areaFile)
		if err != nil {
			return nil, fmt.Errorf("failed to parse area %s: %w", slug, err)
		}

		areas = append(areas, area)
	}

	return areas, nil
}

// Update saves changes to an existing area
func (r *AreaRepo) Update(ctx context.Context, area *domain.Area) error {
	if err := area.Validate(); err != nil {
		return err
	}

	// Find existing area to get slug
	existing, err := r.Get(ctx, area.ID)
	if err != nil {
		return err
	}

	area.UpdateTimestamp()

	// If title changed, we might need to rename the directory
	oldSlug := existing.Slug()
	newSlug := area.Slug()

	if oldSlug != newSlug {
		// Rename directory
		oldDir := r.areaDir(oldSlug)
		newDir := r.areaDir(newSlug)
		if err := os.Rename(oldDir, newDir); err != nil {
			return fmt.Errorf("failed to rename area directory: %w", err)
		}
	}

	// Write updated area file
	if err := r.store.writer.WriteAreaToFile(r.areaFile(newSlug), area); err != nil {
		return err
	}
	r.store.commit(fmt.Sprintf("update area: %s", area.Title))
	return nil
}

// Delete removes an area by ID
func (r *AreaRepo) Delete(ctx context.Context, id string) error {
	area, err := r.Get(ctx, id)
	if err != nil {
		return err
	}

	areaDir := r.areaDir(area.Slug())
	if err := os.RemoveAll(areaDir); err != nil {
		return err
	}
	r.store.commit(fmt.Sprintf("delete area: %s", area.Title))
	return nil
}

// ProjectRepository implementation

// ProjectRepo implements storage.ProjectRepository
type ProjectRepo struct {
	store *Store
}

// NewProjectRepo creates a new ProjectRepo
func (s *Store) Projects() *ProjectRepo {
	return &ProjectRepo{store: s}
}

func (r *ProjectRepo) projectDir(areaSlug, projectSlug string) string {
	return filepath.Join(r.store.rootDir, "areas", areaSlug, "projects", projectSlug)
}

func (r *ProjectRepo) projectFile(areaSlug, projectSlug string) string {
	return filepath.Join(r.projectDir(areaSlug, projectSlug), projectSlug+".md")
}

// Create stores a new project
func (r *ProjectRepo) Create(ctx context.Context, project *domain.Project) error {
	if err := project.Validate(); err != nil {
		return err
	}

	// Get area to find slug
	area, err := r.store.Areas().Get(ctx, project.AreaID)
	if err != nil {
		return fmt.Errorf("area not found: %w", err)
	}

	areaSlug := area.Slug()
	projectSlug := project.Slug()
	projectDir := r.projectDir(areaSlug, projectSlug)

	// Check if project already exists
	if _, err := os.Stat(projectDir); err == nil {
		return fmt.Errorf("project '%s' already exists in area '%s'", projectSlug, areaSlug)
	}

	// Create project directory structure
	tasksDir := filepath.Join(projectDir, "tasks")
	if err := os.MkdirAll(tasksDir, 0755); err != nil {
		return fmt.Errorf("failed to create project directory: %w", err)
	}

	// Write project file
	if err := r.store.writer.WriteProjectToFile(r.projectFile(areaSlug, projectSlug), project); err != nil {
		_ = os.RemoveAll(projectDir)
		return err
	}

	r.store.commit(fmt.Sprintf("create project: %s", project.Title))
	return nil
}

// Get retrieves a project by ID
func (r *ProjectRepo) Get(ctx context.Context, id string) (*domain.Project, error) {
	projects, err := r.ListAll(ctx)
	if err != nil {
		return nil, err
	}

	for _, project := range projects {
		if project.ID == id {
			return project, nil
		}
	}

	return nil, fmt.Errorf("project not found: %s", id)
}

// GetBySlug retrieves a project by its slug within an area
func (r *ProjectRepo) GetBySlug(ctx context.Context, areaSlug, projectSlug string) (*domain.Project, error) {
	projectFile := r.projectFile(areaSlug, projectSlug)
	if _, err := os.Stat(projectFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("project not found: %s/%s", areaSlug, projectSlug)
	}

	return r.store.parser.ParseProjectFromFile(projectFile)
}

// List returns all projects for an area
func (r *ProjectRepo) List(ctx context.Context, areaID string) ([]*domain.Project, error) {
	area, err := r.store.Areas().Get(ctx, areaID)
	if err != nil {
		return nil, err
	}

	return r.listByAreaSlug(ctx, area.Slug())
}

func (r *ProjectRepo) listByAreaSlug(ctx context.Context, areaSlug string) ([]*domain.Project, error) {
	projectsDir := filepath.Join(r.store.rootDir, "areas", areaSlug, "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*domain.Project{}, nil
		}
		return nil, fmt.Errorf("failed to read projects directory: %w", err)
	}

	var projects []*domain.Project
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		projectSlug := entry.Name()
		projectFile := r.projectFile(areaSlug, projectSlug)

		if _, err := os.Stat(projectFile); os.IsNotExist(err) {
			continue
		}

		project, err := r.store.parser.ParseProjectFromFile(projectFile)
		if err != nil {
			return nil, fmt.Errorf("failed to parse project %s: %w", projectSlug, err)
		}

		projects = append(projects, project)
	}

	return projects, nil
}

// ListAll returns all projects across all areas
func (r *ProjectRepo) ListAll(ctx context.Context) ([]*domain.Project, error) {
	areas, err := r.store.Areas().List(ctx)
	if err != nil {
		return nil, err
	}

	var allProjects []*domain.Project
	for _, area := range areas {
		projects, err := r.listByAreaSlug(ctx, area.Slug())
		if err != nil {
			return nil, err
		}
		allProjects = append(allProjects, projects...)
	}

	return allProjects, nil
}

// Update saves changes to an existing project
func (r *ProjectRepo) Update(ctx context.Context, project *domain.Project) error {
	if err := project.Validate(); err != nil {
		return err
	}

	existing, err := r.Get(ctx, project.ID)
	if err != nil {
		return err
	}

	project.UpdateTimestamp()

	// Get area slug
	area, err := r.store.Areas().Get(ctx, project.AreaID)
	if err != nil {
		return err
	}
	areaSlug := area.Slug()

	// Handle potential slug change
	oldSlug := existing.Slug()
	newSlug := project.Slug()

	if oldSlug != newSlug {
		oldDir := r.projectDir(areaSlug, oldSlug)
		newDir := r.projectDir(areaSlug, newSlug)
		if err := os.Rename(oldDir, newDir); err != nil {
			return fmt.Errorf("failed to rename project directory: %w", err)
		}
	}

	if err := r.store.writer.WriteProjectToFile(r.projectFile(areaSlug, newSlug), project); err != nil {
		return err
	}
	r.store.commit(fmt.Sprintf("update project: %s", project.Title))
	return nil
}

// Delete removes a project by ID
func (r *ProjectRepo) Delete(ctx context.Context, id string) error {
	project, err := r.Get(ctx, id)
	if err != nil {
		return err
	}

	area, err := r.store.Areas().Get(ctx, project.AreaID)
	if err != nil {
		return err
	}

	projectDir := r.projectDir(area.Slug(), project.Slug())
	if err := os.RemoveAll(projectDir); err != nil {
		return err
	}
	r.store.commit(fmt.Sprintf("delete project: %s", project.Title))
	return nil
}

// TaskRepository implementation

// TaskRepo implements storage.TaskRepository
type TaskRepo struct {
	store *Store
}

// NewTaskRepo creates a new TaskRepo
func (s *Store) Tasks() *TaskRepo {
	return &TaskRepo{store: s}
}

func (r *TaskRepo) taskFile(areaSlug, projectSlug, taskSlug string) string {
	return filepath.Join(r.store.rootDir, "areas", areaSlug, "projects", projectSlug, "tasks", taskSlug+".md")
}

// Create stores a new task
func (r *TaskRepo) Create(ctx context.Context, task *domain.Task) error {
	if err := task.Validate(); err != nil {
		return err
	}

	// Get project and area slugs
	project, err := r.store.Projects().Get(ctx, task.ProjectID)
	if err != nil {
		return fmt.Errorf("project not found: %w", err)
	}

	area, err := r.store.Areas().Get(ctx, task.AreaID)
	if err != nil {
		return fmt.Errorf("area not found: %w", err)
	}

	taskFile := r.taskFile(area.Slug(), project.Slug(), task.Slug())

	// Check if task already exists
	if _, err := os.Stat(taskFile); err == nil {
		return fmt.Errorf("task '%s' already exists", task.Slug())
	}

	if err := r.store.writer.WriteTaskToFile(taskFile, task); err != nil {
		return err
	}
	r.store.commit(fmt.Sprintf("create task: %s", task.Title))
	return nil
}

// Get retrieves a task by ID
func (r *TaskRepo) Get(ctx context.Context, id string) (*domain.Task, error) {
	tasks, err := r.ListAll(ctx)
	if err != nil {
		return nil, err
	}

	for _, task := range tasks {
		if task.ID == id {
			return task, nil
		}
	}

	return nil, fmt.Errorf("task not found: %s", id)
}

// GetBySlug retrieves a task by its slug within a project
func (r *TaskRepo) GetBySlug(ctx context.Context, areaSlug, projectSlug, taskSlug string) (*domain.Task, error) {
	taskFile := r.taskFile(areaSlug, projectSlug, taskSlug)
	if _, err := os.Stat(taskFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("task not found: %s/%s/%s", areaSlug, projectSlug, taskSlug)
	}

	return r.store.parser.ParseTaskFromFile(taskFile)
}

// List returns all tasks for a project
func (r *TaskRepo) List(ctx context.Context, projectID string) ([]*domain.Task, error) {
	project, err := r.store.Projects().Get(ctx, projectID)
	if err != nil {
		return nil, err
	}

	area, err := r.store.Areas().Get(ctx, project.AreaID)
	if err != nil {
		return nil, err
	}

	return r.listByProjectSlug(ctx, area.Slug(), project.Slug())
}

func (r *TaskRepo) listByProjectSlug(ctx context.Context, areaSlug, projectSlug string) ([]*domain.Task, error) {
	tasksDir := filepath.Join(r.store.rootDir, "areas", areaSlug, "projects", projectSlug, "tasks")
	entries, err := os.ReadDir(tasksDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*domain.Task{}, nil
		}
		return nil, fmt.Errorf("failed to read tasks directory: %w", err)
	}

	var tasks []*domain.Task
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		taskSlug := strings.TrimSuffix(entry.Name(), ".md")
		taskFile := r.taskFile(areaSlug, projectSlug, taskSlug)

		task, err := r.store.parser.ParseTaskFromFile(taskFile)
		if err != nil {
			return nil, fmt.Errorf("failed to parse task %s: %w", taskSlug, err)
		}

		tasks = append(tasks, task)
	}

	return tasks, nil
}

// ListByArea returns all tasks for an area
func (r *TaskRepo) ListByArea(ctx context.Context, areaID string) ([]*domain.Task, error) {
	projects, err := r.store.Projects().List(ctx, areaID)
	if err != nil {
		return nil, err
	}

	var allTasks []*domain.Task
	for _, project := range projects {
		tasks, err := r.List(ctx, project.ID)
		if err != nil {
			return nil, err
		}
		allTasks = append(allTasks, tasks...)
	}

	return allTasks, nil
}

// ListAll returns all tasks
func (r *TaskRepo) ListAll(ctx context.Context) ([]*domain.Task, error) {
	areas, err := r.store.Areas().List(ctx)
	if err != nil {
		return nil, err
	}

	var allTasks []*domain.Task
	for _, area := range areas {
		tasks, err := r.ListByArea(ctx, area.ID)
		if err != nil {
			return nil, err
		}
		allTasks = append(allTasks, tasks...)
	}

	return allTasks, nil
}

// Update saves changes to an existing task
func (r *TaskRepo) Update(ctx context.Context, task *domain.Task) error {
	if err := task.Validate(); err != nil {
		return err
	}

	existing, err := r.Get(ctx, task.ID)
	if err != nil {
		return err
	}

	task.UpdateTimestamp()

	project, err := r.store.Projects().Get(ctx, task.ProjectID)
	if err != nil {
		return err
	}

	area, err := r.store.Areas().Get(ctx, task.AreaID)
	if err != nil {
		return err
	}

	areaSlug := area.Slug()
	projectSlug := project.Slug()

	oldSlug := existing.Slug()
	newSlug := task.Slug()

	if oldSlug != newSlug {
		oldFile := r.taskFile(areaSlug, projectSlug, oldSlug)
		if err := os.Remove(oldFile); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove old task file: %w", err)
		}
	}

	if err := r.store.writer.WriteTaskToFile(r.taskFile(areaSlug, projectSlug, newSlug), task); err != nil {
		return err
	}
	r.store.commit(fmt.Sprintf("update task: %s", task.Title))
	return nil
}

// Delete removes a task by ID
func (r *TaskRepo) Delete(ctx context.Context, id string) error {
	task, err := r.Get(ctx, id)
	if err != nil {
		return err
	}

	project, err := r.store.Projects().Get(ctx, task.ProjectID)
	if err != nil {
		return err
	}

	area, err := r.store.Areas().Get(ctx, task.AreaID)
	if err != nil {
		return err
	}

	taskFile := r.taskFile(area.Slug(), project.Slug(), task.Slug())
	if err := os.Remove(taskFile); err != nil {
		return err
	}
	r.store.commit(fmt.Sprintf("delete task: %s", task.Title))
	return nil
}

// Ensure implementations satisfy interfaces
var (
	_ storage.AreaRepository    = (*AreaRepo)(nil)
	_ storage.ProjectRepository = (*ProjectRepo)(nil)
	_ storage.TaskRepository    = (*TaskRepo)(nil)
)
