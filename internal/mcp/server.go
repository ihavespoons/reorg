package mcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ihavespoons/reorg/internal/domain"
	"github.com/ihavespoons/reorg/internal/service"
)

// Server wraps the MCP server with reorg functionality
type Server struct {
	server *mcp.Server
	client service.ReorgClient
}

// NewServer creates a new MCP server with all reorg tools
func NewServer(client service.ReorgClient) *Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "reorg",
		Version: "1.0.0",
	}, nil)

	s := &Server{
		server: server,
		client: client,
	}

	s.registerTools()

	return s
}

// Run starts the MCP server over stdio
func (s *Server) Run(ctx context.Context) error {
	return s.server.Run(ctx, &mcp.StdioTransport{})
}

// registerTools adds all reorg tools to the server
func (s *Server) registerTools() {
	// Area tools
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "list_areas",
		Description: "List all areas (work, personal, life-admin)",
	}, s.listAreas)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "create_area",
		Description: "Create a new area",
	}, s.createArea)

	// Project tools
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "list_projects",
		Description: "List all projects, optionally filtered by area",
	}, s.listProjects)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "create_project",
		Description: "Create a new project in an area",
	}, s.createProject)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "complete_project",
		Description: "Mark a project as completed",
	}, s.completeProject)

	// Task tools
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "list_tasks",
		Description: "List tasks, optionally filtered by project or area",
	}, s.listTasks)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "create_task",
		Description: "Create a new task in a project",
	}, s.createTask)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "complete_task",
		Description: "Mark a task as completed",
	}, s.completeTask)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "start_task",
		Description: "Mark a task as in progress",
	}, s.startTask)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "get_status",
		Description: "Get an overview of all areas, projects, and tasks",
	}, s.getStatus)
}

// Tool input/output types

type EmptyInput struct{}

type ListAreasOutput struct {
	Areas []AreaInfo `json:"areas"`
}

type AreaInfo struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Slug         string `json:"slug"`
	ProjectCount int    `json:"project_count"`
}

func (s *Server) listAreas(ctx context.Context, req *mcp.CallToolRequest, input EmptyInput) (*mcp.CallToolResult, ListAreasOutput, error) {
	areas, err := s.client.ListAreas(ctx)
	if err != nil {
		return nil, ListAreasOutput{}, err
	}

	output := ListAreasOutput{Areas: make([]AreaInfo, len(areas))}
	for i, a := range areas {
		projects, _ := s.client.ListProjects(ctx, a.ID)
		output.Areas[i] = AreaInfo{
			ID:           a.ID,
			Title:        a.Title,
			Slug:         a.Slug(),
			ProjectCount: len(projects),
		}
	}

	return nil, output, nil
}

type CreateAreaInput struct {
	Title string `json:"title" jsonschema:"required,description=The title for the new area"`
}

type CreateAreaOutput struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Slug  string `json:"slug"`
}

func (s *Server) createArea(ctx context.Context, req *mcp.CallToolRequest, input CreateAreaInput) (*mcp.CallToolResult, CreateAreaOutput, error) {
	area := domain.NewArea(input.Title)
	created, err := s.client.CreateArea(ctx, area)
	if err != nil {
		return nil, CreateAreaOutput{}, err
	}

	return nil, CreateAreaOutput{
		ID:    created.ID,
		Title: created.Title,
		Slug:  created.Slug(),
	}, nil
}

type ListProjectsInput struct {
	Area string `json:"area,omitempty" jsonschema:"description=Filter by area slug (optional)"`
}

type ListProjectsOutput struct {
	Projects []ProjectInfo `json:"projects"`
}

type ProjectInfo struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Slug      string `json:"slug"`
	AreaID    string `json:"area_id"`
	AreaTitle string `json:"area_title"`
	Status    string `json:"status"`
	TaskCount int    `json:"task_count"`
}

func (s *Server) listProjects(ctx context.Context, req *mcp.CallToolRequest, input ListProjectsInput) (*mcp.CallToolResult, ListProjectsOutput, error) {
	var projects []*domain.Project
	var err error

	if input.Area != "" {
		area, err := s.client.GetAreaBySlug(ctx, input.Area)
		if err != nil {
			return nil, ListProjectsOutput{}, fmt.Errorf("area not found: %s", input.Area)
		}
		projects, err = s.client.ListProjects(ctx, area.ID)
		if err != nil {
			return nil, ListProjectsOutput{}, err
		}
	} else {
		projects, err = s.client.ListAllProjects(ctx)
		if err != nil {
			return nil, ListProjectsOutput{}, err
		}
	}

	output := ListProjectsOutput{Projects: make([]ProjectInfo, len(projects))}
	for i, p := range projects {
		area, _ := s.client.GetArea(ctx, p.AreaID)
		areaTitle := ""
		if area != nil {
			areaTitle = area.Title
		}
		tasks, _ := s.client.ListTasks(ctx, p.ID)
		output.Projects[i] = ProjectInfo{
			ID:        p.ID,
			Title:     p.Title,
			Slug:      p.Slug(),
			AreaID:    p.AreaID,
			AreaTitle: areaTitle,
			Status:    string(p.Status),
			TaskCount: len(tasks),
		}
	}

	return nil, output, nil
}

type CreateProjectInput struct {
	Title   string `json:"title" jsonschema:"required,description=The title for the new project"`
	Area    string `json:"area" jsonschema:"required,description=The area slug (e.g. work or personal or life-admin)"`
	Content string `json:"content,omitempty" jsonschema:"description=Optional description or notes for the project"`
}

type CreateProjectOutput struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Slug  string `json:"slug"`
	Area  string `json:"area"`
}

func (s *Server) createProject(ctx context.Context, req *mcp.CallToolRequest, input CreateProjectInput) (*mcp.CallToolResult, CreateProjectOutput, error) {
	area, err := s.client.GetAreaBySlug(ctx, input.Area)
	if err != nil {
		return nil, CreateProjectOutput{}, fmt.Errorf("area not found: %s", input.Area)
	}

	project := domain.NewProject(input.Title, area.ID)
	project.Content = input.Content

	created, err := s.client.CreateProject(ctx, project)
	if err != nil {
		return nil, CreateProjectOutput{}, err
	}

	return nil, CreateProjectOutput{
		ID:    created.ID,
		Title: created.Title,
		Slug:  created.Slug(),
		Area:  area.Title,
	}, nil
}

type CompleteProjectInput struct {
	ID string `json:"id" jsonschema:"required,description=The project ID to complete"`
}

type CompleteProjectOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func (s *Server) completeProject(ctx context.Context, req *mcp.CallToolRequest, input CompleteProjectInput) (*mcp.CallToolResult, CompleteProjectOutput, error) {
	if err := s.client.CompleteProject(ctx, input.ID); err != nil {
		return nil, CompleteProjectOutput{Success: false, Message: err.Error()}, nil
	}

	return nil, CompleteProjectOutput{
		Success: true,
		Message: "Project marked as completed",
	}, nil
}

type ListTasksInput struct {
	Project string `json:"project,omitempty" jsonschema:"description=Filter by project ID (optional)"`
	Area    string `json:"area,omitempty" jsonschema:"description=Filter by area slug (optional)"`
	Status  string `json:"status,omitempty" jsonschema:"description=Filter by status: pending, in_progress, completed, blocked (optional)"`
}

type ListTasksOutput struct {
	Tasks []TaskInfo `json:"tasks"`
}

type TaskInfo struct {
	ID           string  `json:"id"`
	Title        string  `json:"title"`
	Status       string  `json:"status"`
	Priority     string  `json:"priority"`
	ProjectID    string  `json:"project_id"`
	ProjectTitle string  `json:"project_title"`
	DueDate      *string `json:"due_date,omitempty"`
	IsOverdue    bool    `json:"is_overdue"`
}

func (s *Server) listTasks(ctx context.Context, req *mcp.CallToolRequest, input ListTasksInput) (*mcp.CallToolResult, ListTasksOutput, error) {
	var tasks []*domain.Task
	var err error

	if input.Project != "" {
		tasks, err = s.client.ListTasks(ctx, input.Project)
	} else if input.Area != "" {
		area, err := s.client.GetAreaBySlug(ctx, input.Area)
		if err != nil {
			return nil, ListTasksOutput{}, fmt.Errorf("area not found: %s", input.Area)
		}
		tasks, err = s.client.ListTasksByArea(ctx, area.ID)
		if err != nil {
			return nil, ListTasksOutput{}, err
		}
	} else {
		tasks, err = s.client.ListAllTasks(ctx)
	}

	if err != nil {
		return nil, ListTasksOutput{}, err
	}

	// Filter by status if specified
	if input.Status != "" {
		filtered := make([]*domain.Task, 0)
		for _, t := range tasks {
			if string(t.Status) == input.Status {
				filtered = append(filtered, t)
			}
		}
		tasks = filtered
	}

	output := ListTasksOutput{Tasks: make([]TaskInfo, len(tasks))}
	for i, t := range tasks {
		projectTitle := ""
		if project, _ := s.client.GetProject(ctx, t.ProjectID); project != nil {
			projectTitle = project.Title
		}

		var dueDate *string
		if t.DueDate != nil {
			d := t.DueDate.Format("2006-01-02")
			dueDate = &d
		}

		output.Tasks[i] = TaskInfo{
			ID:           t.ID,
			Title:        t.Title,
			Status:       string(t.Status),
			Priority:     string(t.Priority),
			ProjectID:    t.ProjectID,
			ProjectTitle: projectTitle,
			DueDate:      dueDate,
			IsOverdue:    t.IsOverdue(),
		}
	}

	return nil, output, nil
}

type CreateTaskInput struct {
	Title       string `json:"title" jsonschema:"required,description=The task title (should be action-oriented)"`
	Project     string `json:"project" jsonschema:"required,description=The project ID to add the task to"`
	Description string `json:"description,omitempty" jsonschema:"description=Optional description or notes"`
	Priority    string `json:"priority,omitempty" jsonschema:"description=Priority: low, medium, high, urgent (default: medium)"`
	DueDate     string `json:"due_date,omitempty" jsonschema:"description=Due date in YYYY-MM-DD format (optional)"`
}

type CreateTaskOutput struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Project  string `json:"project"`
	Priority string `json:"priority"`
}

func (s *Server) createTask(ctx context.Context, req *mcp.CallToolRequest, input CreateTaskInput) (*mcp.CallToolResult, CreateTaskOutput, error) {
	project, err := s.client.GetProject(ctx, input.Project)
	if err != nil {
		return nil, CreateTaskOutput{}, fmt.Errorf("project not found: %s", input.Project)
	}

	task := domain.NewTask(input.Title, project.ID, project.AreaID)
	task.Content = input.Description

	if input.Priority != "" {
		switch strings.ToLower(input.Priority) {
		case "low":
			task.Priority = domain.PriorityLow
		case "high":
			task.Priority = domain.PriorityHigh
		case "urgent":
			task.Priority = domain.PriorityUrgent
		default:
			task.Priority = domain.PriorityMedium
		}
	}

	if input.DueDate != "" {
		if due, err := time.Parse("2006-01-02", input.DueDate); err == nil {
			task.DueDate = &due
		}
	}

	created, err := s.client.CreateTask(ctx, task)
	if err != nil {
		return nil, CreateTaskOutput{}, err
	}

	return nil, CreateTaskOutput{
		ID:       created.ID,
		Title:    created.Title,
		Project:  project.Title,
		Priority: string(created.Priority),
	}, nil
}

type CompleteTaskInput struct {
	ID string `json:"id" jsonschema:"required,description=The task ID to complete"`
}

type CompleteTaskOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func (s *Server) completeTask(ctx context.Context, req *mcp.CallToolRequest, input CompleteTaskInput) (*mcp.CallToolResult, CompleteTaskOutput, error) {
	if err := s.client.CompleteTask(ctx, input.ID); err != nil {
		return nil, CompleteTaskOutput{Success: false, Message: err.Error()}, nil
	}

	return nil, CompleteTaskOutput{
		Success: true,
		Message: "Task marked as completed",
	}, nil
}

type StartTaskInput struct {
	ID string `json:"id" jsonschema:"required,description=The task ID to start"`
}

type StartTaskOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func (s *Server) startTask(ctx context.Context, req *mcp.CallToolRequest, input StartTaskInput) (*mcp.CallToolResult, StartTaskOutput, error) {
	if err := s.client.StartTask(ctx, input.ID); err != nil {
		return nil, StartTaskOutput{Success: false, Message: err.Error()}, nil
	}

	return nil, StartTaskOutput{
		Success: true,
		Message: "Task marked as in progress",
	}, nil
}

type StatusOutput struct {
	Summary string       `json:"summary"`
	Areas   []AreaStatus `json:"areas"`
}

type AreaStatus struct {
	Title    string          `json:"title"`
	Projects []ProjectStatus `json:"projects"`
}

type ProjectStatus struct {
	Title         string `json:"title"`
	Status        string `json:"status"`
	TotalTasks    int    `json:"total_tasks"`
	PendingTasks  int    `json:"pending_tasks"`
	InProgress    int    `json:"in_progress"`
	CompletedTasks int   `json:"completed_tasks"`
}

func (s *Server) getStatus(ctx context.Context, req *mcp.CallToolRequest, input EmptyInput) (*mcp.CallToolResult, StatusOutput, error) {
	areas, err := s.client.ListAreas(ctx)
	if err != nil {
		return nil, StatusOutput{}, err
	}

	output := StatusOutput{
		Areas: make([]AreaStatus, len(areas)),
	}

	totalProjects := 0
	totalTasks := 0
	totalPending := 0
	totalInProgress := 0

	for i, area := range areas {
		projects, _ := s.client.ListProjects(ctx, area.ID)
		areaStatus := AreaStatus{
			Title:    area.Title,
			Projects: make([]ProjectStatus, len(projects)),
		}

		for j, p := range projects {
			tasks, _ := s.client.ListTasks(ctx, p.ID)
			ps := ProjectStatus{
				Title:      p.Title,
				Status:     string(p.Status),
				TotalTasks: len(tasks),
			}

			for _, t := range tasks {
				switch t.Status {
				case domain.TaskStatusPending:
					ps.PendingTasks++
					totalPending++
				case domain.TaskStatusInProgress:
					ps.InProgress++
					totalInProgress++
				case domain.TaskStatusCompleted:
					ps.CompletedTasks++
				}
			}

			areaStatus.Projects[j] = ps
			totalTasks += len(tasks)
		}

		output.Areas[i] = areaStatus
		totalProjects += len(projects)
	}

	output.Summary = fmt.Sprintf("%d areas, %d projects, %d tasks (%d pending, %d in progress)",
		len(areas), totalProjects, totalTasks, totalPending, totalInProgress)

	return nil, output, nil
}

