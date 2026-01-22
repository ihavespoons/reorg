package client

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/beng/reorg/api/proto/gen"
	"github.com/beng/reorg/internal/domain"
	"github.com/beng/reorg/internal/service"
)

// RemoteClient implements ReorgClient by connecting via gRPC
type RemoteClient struct {
	conn   *grpc.ClientConn
	client pb.ReorgServiceClient
}

// NewRemoteClient creates a new remote client connected to the given address
func NewRemoteClient(address string) (*RemoteClient, error) {
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to server: %w", err)
	}

	return &RemoteClient{
		conn:   conn,
		client: pb.NewReorgServiceClient(conn),
	}, nil
}

// Close closes the gRPC connection
func (c *RemoteClient) Close() error {
	return c.conn.Close()
}

// AreaService implementation

func (c *RemoteClient) CreateArea(ctx context.Context, area *domain.Area) (*domain.Area, error) {
	resp, err := c.client.CreateArea(ctx, &pb.CreateAreaRequest{
		Title:   area.Title,
		Content: area.Content,
	})
	if err != nil {
		return nil, err
	}
	return protoToArea(resp.Area), nil
}

func (c *RemoteClient) GetArea(ctx context.Context, id string) (*domain.Area, error) {
	resp, err := c.client.GetArea(ctx, &pb.GetAreaRequest{Id: id})
	if err != nil {
		return nil, err
	}
	return protoToArea(resp.Area), nil
}

func (c *RemoteClient) GetAreaBySlug(ctx context.Context, slug string) (*domain.Area, error) {
	// Remote client needs to list all areas and find by slug
	// This could be optimized with a dedicated RPC call
	areas, err := c.ListAreas(ctx)
	if err != nil {
		return nil, err
	}
	for _, area := range areas {
		if area.Slug() == slug {
			return area, nil
		}
	}
	return nil, fmt.Errorf("area not found: %s", slug)
}

func (c *RemoteClient) ListAreas(ctx context.Context) ([]*domain.Area, error) {
	resp, err := c.client.ListAreas(ctx, &pb.ListAreasRequest{})
	if err != nil {
		return nil, err
	}

	areas := make([]*domain.Area, len(resp.Areas))
	for i, a := range resp.Areas {
		areas[i] = protoToArea(a)
	}
	return areas, nil
}

func (c *RemoteClient) UpdateArea(ctx context.Context, area *domain.Area) error {
	_, err := c.client.UpdateArea(ctx, &pb.UpdateAreaRequest{
		Area: areaToProto(area),
	})
	return err
}

func (c *RemoteClient) DeleteArea(ctx context.Context, id string) error {
	_, err := c.client.DeleteArea(ctx, &pb.DeleteAreaRequest{Id: id})
	return err
}

// ProjectService implementation

func (c *RemoteClient) CreateProject(ctx context.Context, project *domain.Project) (*domain.Project, error) {
	req := &pb.CreateProjectRequest{
		Title:   project.Title,
		AreaId:  project.AreaID,
		Content: project.Content,
		Tags:    project.Tags,
	}
	if project.DueDate != nil {
		req.DueDate = timestamppb.New(*project.DueDate)
	}

	resp, err := c.client.CreateProject(ctx, req)
	if err != nil {
		return nil, err
	}
	return protoToProject(resp.Project), nil
}

func (c *RemoteClient) GetProject(ctx context.Context, id string) (*domain.Project, error) {
	resp, err := c.client.GetProject(ctx, &pb.GetProjectRequest{Id: id})
	if err != nil {
		return nil, err
	}
	return protoToProject(resp.Project), nil
}

func (c *RemoteClient) GetProjectBySlug(ctx context.Context, areaID, slug string) (*domain.Project, error) {
	// Get projects in area and find by slug
	projects, err := c.ListProjects(ctx, areaID)
	if err != nil {
		return nil, err
	}
	for _, project := range projects {
		if project.Slug() == slug {
			return project, nil
		}
	}
	return nil, fmt.Errorf("project not found: %s", slug)
}

func (c *RemoteClient) ListProjects(ctx context.Context, areaID string) ([]*domain.Project, error) {
	resp, err := c.client.ListProjects(ctx, &pb.ListProjectsRequest{AreaId: areaID})
	if err != nil {
		return nil, err
	}

	projects := make([]*domain.Project, len(resp.Projects))
	for i, p := range resp.Projects {
		projects[i] = protoToProject(p)
	}
	return projects, nil
}

func (c *RemoteClient) ListAllProjects(ctx context.Context) ([]*domain.Project, error) {
	resp, err := c.client.ListProjects(ctx, &pb.ListProjectsRequest{})
	if err != nil {
		return nil, err
	}

	projects := make([]*domain.Project, len(resp.Projects))
	for i, p := range resp.Projects {
		projects[i] = protoToProject(p)
	}
	return projects, nil
}

func (c *RemoteClient) UpdateProject(ctx context.Context, project *domain.Project) error {
	_, err := c.client.UpdateProject(ctx, &pb.UpdateProjectRequest{
		Project: projectToProto(project),
	})
	return err
}

func (c *RemoteClient) DeleteProject(ctx context.Context, id string) error {
	_, err := c.client.DeleteProject(ctx, &pb.DeleteProjectRequest{Id: id})
	return err
}

func (c *RemoteClient) CompleteProject(ctx context.Context, id string) error {
	_, err := c.client.CompleteProject(ctx, &pb.CompleteProjectRequest{Id: id})
	return err
}

// TaskService implementation

func (c *RemoteClient) CreateTask(ctx context.Context, task *domain.Task) (*domain.Task, error) {
	req := &pb.CreateTaskRequest{
		Title:     task.Title,
		ProjectId: task.ProjectID,
		AreaId:    task.AreaID,
		Content:   task.Content,
		Priority:  priorityToProto(task.Priority),
		Tags:      task.Tags,
	}
	if task.DueDate != nil {
		req.DueDate = timestamppb.New(*task.DueDate)
	}

	resp, err := c.client.CreateTask(ctx, req)
	if err != nil {
		return nil, err
	}
	return protoToTask(resp.Task), nil
}

func (c *RemoteClient) GetTask(ctx context.Context, id string) (*domain.Task, error) {
	resp, err := c.client.GetTask(ctx, &pb.GetTaskRequest{Id: id})
	if err != nil {
		return nil, err
	}
	return protoToTask(resp.Task), nil
}

func (c *RemoteClient) GetTaskBySlug(ctx context.Context, projectID, slug string) (*domain.Task, error) {
	tasks, err := c.ListTasks(ctx, projectID)
	if err != nil {
		return nil, err
	}
	for _, task := range tasks {
		if task.Slug() == slug {
			return task, nil
		}
	}
	return nil, fmt.Errorf("task not found: %s", slug)
}

func (c *RemoteClient) ListTasks(ctx context.Context, projectID string) ([]*domain.Task, error) {
	resp, err := c.client.ListTasks(ctx, &pb.ListTasksRequest{ProjectId: projectID})
	if err != nil {
		return nil, err
	}

	tasks := make([]*domain.Task, len(resp.Tasks))
	for i, t := range resp.Tasks {
		tasks[i] = protoToTask(t)
	}
	return tasks, nil
}

func (c *RemoteClient) ListTasksByArea(ctx context.Context, areaID string) ([]*domain.Task, error) {
	resp, err := c.client.ListTasks(ctx, &pb.ListTasksRequest{AreaId: areaID})
	if err != nil {
		return nil, err
	}

	tasks := make([]*domain.Task, len(resp.Tasks))
	for i, t := range resp.Tasks {
		tasks[i] = protoToTask(t)
	}
	return tasks, nil
}

func (c *RemoteClient) ListAllTasks(ctx context.Context) ([]*domain.Task, error) {
	resp, err := c.client.ListTasks(ctx, &pb.ListTasksRequest{})
	if err != nil {
		return nil, err
	}

	tasks := make([]*domain.Task, len(resp.Tasks))
	for i, t := range resp.Tasks {
		tasks[i] = protoToTask(t)
	}
	return tasks, nil
}

func (c *RemoteClient) UpdateTask(ctx context.Context, task *domain.Task) error {
	_, err := c.client.UpdateTask(ctx, &pb.UpdateTaskRequest{
		Task: taskToProto(task),
	})
	return err
}

func (c *RemoteClient) DeleteTask(ctx context.Context, id string) error {
	_, err := c.client.DeleteTask(ctx, &pb.DeleteTaskRequest{Id: id})
	return err
}

func (c *RemoteClient) StartTask(ctx context.Context, id string) error {
	_, err := c.client.StartTask(ctx, &pb.StartTaskRequest{Id: id})
	return err
}

func (c *RemoteClient) CompleteTask(ctx context.Context, id string) error {
	_, err := c.client.CompleteTask(ctx, &pb.CompleteTaskRequest{Id: id})
	return err
}

// Conversion helpers

func areaToProto(a *domain.Area) *pb.Area {
	return &pb.Area{
		Id:        a.ID,
		Title:     a.Title,
		Content:   a.Content,
		CreatedAt: timestamppb.New(a.Created),
		UpdatedAt: timestamppb.New(a.Updated),
	}
}

func protoToArea(p *pb.Area) *domain.Area {
	return &domain.Area{
		ID:      p.Id,
		Title:   p.Title,
		Type:    "area",
		Content: p.Content,
		Timestamps: domain.Timestamps{
			Created: p.CreatedAt.AsTime(),
			Updated: p.UpdatedAt.AsTime(),
		},
	}
}

func projectToProto(p *domain.Project) *pb.Project {
	proj := &pb.Project{
		Id:        p.ID,
		Title:     p.Title,
		AreaId:    p.AreaID,
		Content:   p.Content,
		Status:    projectStatusToProto(p.Status),
		Tags:      p.Tags,
		CreatedAt: timestamppb.New(p.Created),
		UpdatedAt: timestamppb.New(p.Updated),
	}
	if p.DueDate != nil {
		proj.DueDate = timestamppb.New(*p.DueDate)
	}
	return proj
}

func protoToProject(p *pb.Project) *domain.Project {
	proj := &domain.Project{
		ID:      p.Id,
		Title:   p.Title,
		Type:    "project",
		AreaID:  p.AreaId,
		Content: p.Content,
		Status:  protoProjectStatusToDomain(p.Status),
		Tags:    p.Tags,
		Timestamps: domain.Timestamps{
			Created: p.CreatedAt.AsTime(),
			Updated: p.UpdatedAt.AsTime(),
		},
	}
	if p.DueDate != nil {
		due := p.DueDate.AsTime()
		proj.DueDate = &due
	}
	return proj
}

func taskToProto(t *domain.Task) *pb.Task {
	task := &pb.Task{
		Id:           t.ID,
		Title:        t.Title,
		ProjectId:    t.ProjectID,
		AreaId:       t.AreaID,
		Content:      t.Content,
		Status:       taskStatusToProto(t.Status),
		Priority:     priorityToProto(t.Priority),
		Tags:         t.Tags,
		Dependencies: t.Dependencies,
		CreatedAt:    timestamppb.New(t.Created),
		UpdatedAt:    timestamppb.New(t.Updated),
	}
	if t.DueDate != nil {
		task.DueDate = timestamppb.New(*t.DueDate)
	}
	return task
}

func protoToTask(p *pb.Task) *domain.Task {
	task := &domain.Task{
		ID:           p.Id,
		Title:        p.Title,
		Type:         "task",
		ProjectID:    p.ProjectId,
		AreaID:       p.AreaId,
		Content:      p.Content,
		Status:       protoTaskStatusToDomain(p.Status),
		Priority:     protoPriorityToDomain(p.Priority),
		Tags:         p.Tags,
		Dependencies: p.Dependencies,
		Timestamps: domain.Timestamps{
			Created: p.CreatedAt.AsTime(),
			Updated: p.UpdatedAt.AsTime(),
		},
	}
	if p.DueDate != nil {
		due := p.DueDate.AsTime()
		task.DueDate = &due
	}
	return task
}

func projectStatusToProto(s domain.ProjectStatus) pb.ProjectStatus {
	switch s {
	case domain.ProjectStatusActive:
		return pb.ProjectStatus_PROJECT_STATUS_ACTIVE
	case domain.ProjectStatusOnHold:
		return pb.ProjectStatus_PROJECT_STATUS_ON_HOLD
	case domain.ProjectStatusCompleted:
		return pb.ProjectStatus_PROJECT_STATUS_COMPLETED
	case domain.ProjectStatusArchived:
		return pb.ProjectStatus_PROJECT_STATUS_ARCHIVED
	default:
		return pb.ProjectStatus_PROJECT_STATUS_UNSPECIFIED
	}
}

func protoProjectStatusToDomain(s pb.ProjectStatus) domain.ProjectStatus {
	switch s {
	case pb.ProjectStatus_PROJECT_STATUS_ACTIVE:
		return domain.ProjectStatusActive
	case pb.ProjectStatus_PROJECT_STATUS_ON_HOLD:
		return domain.ProjectStatusOnHold
	case pb.ProjectStatus_PROJECT_STATUS_COMPLETED:
		return domain.ProjectStatusCompleted
	case pb.ProjectStatus_PROJECT_STATUS_ARCHIVED:
		return domain.ProjectStatusArchived
	default:
		return domain.ProjectStatusActive
	}
}

func taskStatusToProto(s domain.TaskStatus) pb.TaskStatus {
	switch s {
	case domain.TaskStatusPending:
		return pb.TaskStatus_TASK_STATUS_TODO
	case domain.TaskStatusInProgress:
		return pb.TaskStatus_TASK_STATUS_IN_PROGRESS
	case domain.TaskStatusBlocked:
		return pb.TaskStatus_TASK_STATUS_BLOCKED
	case domain.TaskStatusCompleted:
		return pb.TaskStatus_TASK_STATUS_DONE
	case domain.TaskStatusCancelled:
		return pb.TaskStatus_TASK_STATUS_CANCELLED
	default:
		return pb.TaskStatus_TASK_STATUS_UNSPECIFIED
	}
}

func protoTaskStatusToDomain(s pb.TaskStatus) domain.TaskStatus {
	switch s {
	case pb.TaskStatus_TASK_STATUS_TODO:
		return domain.TaskStatusPending
	case pb.TaskStatus_TASK_STATUS_IN_PROGRESS:
		return domain.TaskStatusInProgress
	case pb.TaskStatus_TASK_STATUS_BLOCKED:
		return domain.TaskStatusBlocked
	case pb.TaskStatus_TASK_STATUS_DONE:
		return domain.TaskStatusCompleted
	case pb.TaskStatus_TASK_STATUS_CANCELLED:
		return domain.TaskStatusCancelled
	default:
		return domain.TaskStatusPending
	}
}

func priorityToProto(p domain.Priority) pb.Priority {
	switch p {
	case domain.PriorityLow:
		return pb.Priority_PRIORITY_LOW
	case domain.PriorityMedium:
		return pb.Priority_PRIORITY_MEDIUM
	case domain.PriorityHigh:
		return pb.Priority_PRIORITY_HIGH
	case domain.PriorityUrgent:
		return pb.Priority_PRIORITY_URGENT
	default:
		return pb.Priority_PRIORITY_UNSPECIFIED
	}
}

func protoPriorityToDomain(p pb.Priority) domain.Priority {
	switch p {
	case pb.Priority_PRIORITY_LOW:
		return domain.PriorityLow
	case pb.Priority_PRIORITY_MEDIUM:
		return domain.PriorityMedium
	case pb.Priority_PRIORITY_HIGH:
		return domain.PriorityHigh
	case pb.Priority_PRIORITY_URGENT:
		return domain.PriorityUrgent
	default:
		return domain.PriorityMedium
	}
}

// Ensure RemoteClient implements ReorgClient
var _ service.ReorgClient = (*RemoteClient)(nil)
