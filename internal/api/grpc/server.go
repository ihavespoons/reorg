package grpc

import (
	"context"
	"fmt"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/ihavespoons/reorg/api/proto/gen"
	"github.com/ihavespoons/reorg/internal/domain"
	"github.com/ihavespoons/reorg/internal/service"
)

// Server implements the gRPC ReorgService
type Server struct {
	pb.UnimplementedReorgServiceServer
	client service.ReorgClient
}

// NewServer creates a new gRPC server
func NewServer(client service.ReorgClient) *Server {
	return &Server{client: client}
}

// Start starts the gRPC server on the given address
func (s *Server) Start(address string) error {
	lis, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterReorgServiceServer(grpcServer, s)

	return grpcServer.Serve(lis)
}

// Area operations

func (s *Server) CreateArea(ctx context.Context, req *pb.CreateAreaRequest) (*pb.CreateAreaResponse, error) {
	area := domain.NewArea(req.Title)
	area.Content = req.Content

	created, err := s.client.CreateArea(ctx, area)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create area: %v", err)
	}

	return &pb.CreateAreaResponse{Area: areaToProto(created)}, nil
}

func (s *Server) GetArea(ctx context.Context, req *pb.GetAreaRequest) (*pb.GetAreaResponse, error) {
	area, err := s.client.GetArea(ctx, req.Id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "area not found: %v", err)
	}

	return &pb.GetAreaResponse{Area: areaToProto(area)}, nil
}

func (s *Server) ListAreas(ctx context.Context, req *pb.ListAreasRequest) (*pb.ListAreasResponse, error) {
	areas, err := s.client.ListAreas(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list areas: %v", err)
	}

	pbAreas := make([]*pb.Area, len(areas))
	for i, a := range areas {
		pbAreas[i] = areaToProto(a)
	}

	return &pb.ListAreasResponse{Areas: pbAreas}, nil
}

func (s *Server) UpdateArea(ctx context.Context, req *pb.UpdateAreaRequest) (*pb.UpdateAreaResponse, error) {
	area := protoToArea(req.Area)
	if err := s.client.UpdateArea(ctx, area); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update area: %v", err)
	}

	updated, err := s.client.GetArea(ctx, area.ID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get updated area: %v", err)
	}

	return &pb.UpdateAreaResponse{Area: areaToProto(updated)}, nil
}

func (s *Server) DeleteArea(ctx context.Context, req *pb.DeleteAreaRequest) (*pb.DeleteAreaResponse, error) {
	if err := s.client.DeleteArea(ctx, req.Id); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete area: %v", err)
	}

	return &pb.DeleteAreaResponse{}, nil
}

// Project operations

func (s *Server) CreateProject(ctx context.Context, req *pb.CreateProjectRequest) (*pb.CreateProjectResponse, error) {
	project := domain.NewProject(req.Title, req.AreaId)
	project.Content = req.Content
	for _, tag := range req.Tags {
		project.AddTag(tag)
	}
	if req.DueDate != nil {
		due := req.DueDate.AsTime()
		project.DueDate = &due
	}

	created, err := s.client.CreateProject(ctx, project)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create project: %v", err)
	}

	return &pb.CreateProjectResponse{Project: projectToProto(created)}, nil
}

func (s *Server) GetProject(ctx context.Context, req *pb.GetProjectRequest) (*pb.GetProjectResponse, error) {
	project, err := s.client.GetProject(ctx, req.Id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "project not found: %v", err)
	}

	return &pb.GetProjectResponse{Project: projectToProto(project)}, nil
}

func (s *Server) ListProjects(ctx context.Context, req *pb.ListProjectsRequest) (*pb.ListProjectsResponse, error) {
	var projects []*domain.Project
	var err error

	if req.AreaId != "" {
		projects, err = s.client.ListProjects(ctx, req.AreaId)
	} else {
		projects, err = s.client.ListAllProjects(ctx)
	}

	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list projects: %v", err)
	}

	pbProjects := make([]*pb.Project, len(projects))
	for i, p := range projects {
		pbProjects[i] = projectToProto(p)
	}

	return &pb.ListProjectsResponse{Projects: pbProjects}, nil
}

func (s *Server) UpdateProject(ctx context.Context, req *pb.UpdateProjectRequest) (*pb.UpdateProjectResponse, error) {
	project := protoToProject(req.Project)
	if err := s.client.UpdateProject(ctx, project); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update project: %v", err)
	}

	updated, err := s.client.GetProject(ctx, project.ID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get updated project: %v", err)
	}

	return &pb.UpdateProjectResponse{Project: projectToProto(updated)}, nil
}

func (s *Server) DeleteProject(ctx context.Context, req *pb.DeleteProjectRequest) (*pb.DeleteProjectResponse, error) {
	if err := s.client.DeleteProject(ctx, req.Id); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete project: %v", err)
	}

	return &pb.DeleteProjectResponse{}, nil
}

func (s *Server) CompleteProject(ctx context.Context, req *pb.CompleteProjectRequest) (*pb.CompleteProjectResponse, error) {
	if err := s.client.CompleteProject(ctx, req.Id); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to complete project: %v", err)
	}

	project, err := s.client.GetProject(ctx, req.Id)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get completed project: %v", err)
	}

	return &pb.CompleteProjectResponse{Project: projectToProto(project)}, nil
}

// Task operations

func (s *Server) CreateTask(ctx context.Context, req *pb.CreateTaskRequest) (*pb.CreateTaskResponse, error) {
	task := domain.NewTask(req.Title, req.ProjectId, req.AreaId)
	task.Content = req.Content
	task.Priority = protoPriorityToDomain(req.Priority)
	for _, tag := range req.Tags {
		task.AddTag(tag)
	}
	if req.DueDate != nil {
		due := req.DueDate.AsTime()
		task.DueDate = &due
	}

	created, err := s.client.CreateTask(ctx, task)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create task: %v", err)
	}

	return &pb.CreateTaskResponse{Task: taskToProto(created)}, nil
}

func (s *Server) GetTask(ctx context.Context, req *pb.GetTaskRequest) (*pb.GetTaskResponse, error) {
	task, err := s.client.GetTask(ctx, req.Id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "task not found: %v", err)
	}

	return &pb.GetTaskResponse{Task: taskToProto(task)}, nil
}

func (s *Server) ListTasks(ctx context.Context, req *pb.ListTasksRequest) (*pb.ListTasksResponse, error) {
	var tasks []*domain.Task
	var err error

	if req.ProjectId != "" {
		tasks, err = s.client.ListTasks(ctx, req.ProjectId)
	} else if req.AreaId != "" {
		tasks, err = s.client.ListTasksByArea(ctx, req.AreaId)
	} else {
		tasks, err = s.client.ListAllTasks(ctx)
	}

	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list tasks: %v", err)
	}

	pbTasks := make([]*pb.Task, len(tasks))
	for i, t := range tasks {
		pbTasks[i] = taskToProto(t)
	}

	return &pb.ListTasksResponse{Tasks: pbTasks}, nil
}

func (s *Server) UpdateTask(ctx context.Context, req *pb.UpdateTaskRequest) (*pb.UpdateTaskResponse, error) {
	task := protoToTask(req.Task)
	if err := s.client.UpdateTask(ctx, task); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update task: %v", err)
	}

	updated, err := s.client.GetTask(ctx, task.ID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get updated task: %v", err)
	}

	return &pb.UpdateTaskResponse{Task: taskToProto(updated)}, nil
}

func (s *Server) DeleteTask(ctx context.Context, req *pb.DeleteTaskRequest) (*pb.DeleteTaskResponse, error) {
	if err := s.client.DeleteTask(ctx, req.Id); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete task: %v", err)
	}

	return &pb.DeleteTaskResponse{}, nil
}

func (s *Server) StartTask(ctx context.Context, req *pb.StartTaskRequest) (*pb.StartTaskResponse, error) {
	if err := s.client.StartTask(ctx, req.Id); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to start task: %v", err)
	}

	task, err := s.client.GetTask(ctx, req.Id)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get started task: %v", err)
	}

	return &pb.StartTaskResponse{Task: taskToProto(task)}, nil
}

func (s *Server) CompleteTask(ctx context.Context, req *pb.CompleteTaskRequest) (*pb.CompleteTaskResponse, error) {
	if err := s.client.CompleteTask(ctx, req.Id); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to complete task: %v", err)
	}

	task, err := s.client.GetTask(ctx, req.Id)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get completed task: %v", err)
	}

	return &pb.CompleteTaskResponse{Task: taskToProto(task)}, nil
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
