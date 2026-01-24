package plugin

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/ihavespoons/reorg/pkg/plugin/proto"
)

// GRPCPluginImpl is the implementation of plugin.GRPCPlugin for reorg plugins.
type GRPCPluginImpl struct {
	plugin.Plugin
	// Impl is the plugin implementation (set by plugin authors).
	Impl Plugin
	// broker is set by the host when creating the client.
	broker *plugin.GRPCBroker
}

// GRPCServer returns the gRPC server for this plugin.
func (p *GRPCPluginImpl) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	pb.RegisterReorgPluginServer(s, &grpcPluginServer{
		impl:   p.Impl,
		broker: broker,
	})
	return nil
}

// GRPCClient returns the gRPC client for this plugin.
func (p *GRPCPluginImpl) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return &grpcPluginClient{
		client: pb.NewReorgPluginClient(c),
		broker: broker,
	}, nil
}

// grpcPluginServer implements the gRPC server for a plugin.
// This runs in the plugin process and wraps the plugin implementation.
type grpcPluginServer struct {
	pb.UnimplementedReorgPluginServer
	impl   Plugin
	broker *plugin.GRPCBroker
	host   HostClient
}

func (s *grpcPluginServer) GetManifest(ctx context.Context, _ *pb.Empty) (*pb.PluginManifest, error) {
	m, err := s.impl.GetManifest(ctx)
	if err != nil {
		return nil, err
	}
	return &pb.PluginManifest{
		Name:         m.Name,
		Version:      m.Version,
		Description:  m.Description,
		Author:       m.Author,
		Schedule:     m.Schedule,
		Capabilities: m.Capabilities,
		ConfigSchema: m.ConfigSchema,
	}, nil
}

func (s *grpcPluginServer) Configure(ctx context.Context, req *pb.ConfigureRequest) (*pb.ConfigureResponse, error) {
	// Start a gRPC server for the host client
	var hostClient HostClient
	if req.HostServerPort > 0 {
		conn, err := s.broker.Dial(req.HostServerPort)
		if err != nil {
			return &pb.ConfigureResponse{
				Success: false,
				Error:   "failed to connect to host: " + err.Error(),
			}, nil
		}
		hostClient = &grpcHostClient{client: pb.NewReorgHostClient(conn)}
	}

	s.host = hostClient
	err := s.impl.Configure(ctx, hostClient, req.Config, req.StateDir)
	if err != nil {
		return &pb.ConfigureResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}
	return &pb.ConfigureResponse{Success: true}, nil
}

func (s *grpcPluginServer) Execute(ctx context.Context, req *pb.ExecuteRequest) (*pb.ExecuteResponse, error) {
	result, err := s.impl.Execute(ctx, &ExecuteParams{
		DryRun: req.DryRun,
		Params: req.Params,
	})
	if err != nil {
		return &pb.ExecuteResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	resp := &pb.ExecuteResponse{
		Success: result.Success,
		Error:   result.Error,
	}

	if result.Summary != nil {
		resp.Summary = &pb.ExecuteSummary{
			ItemsProcessed: int32(result.Summary.ItemsProcessed),
			ItemsImported:  int32(result.Summary.ItemsImported),
			ItemsSkipped:   int32(result.Summary.ItemsSkipped),
			ItemsFailed:    int32(result.Summary.ItemsFailed),
			Message:        result.Summary.Message,
		}
	}

	for _, item := range result.Results {
		resp.Results = append(resp.Results, &pb.ExecuteResult{
			Id:       item.ID,
			Name:     item.Name,
			Action:   item.Action,
			Message:  item.Message,
			Metadata: item.Metadata,
		})
	}

	return resp, nil
}

func (s *grpcPluginServer) Shutdown(ctx context.Context, _ *pb.Empty) (*pb.Empty, error) {
	return &pb.Empty{}, s.impl.Shutdown(ctx)
}

// grpcPluginClient implements the gRPC client for a plugin.
// This runs in the host process and communicates with the plugin.
type grpcPluginClient struct {
	client pb.ReorgPluginClient
	broker *plugin.GRPCBroker
}

func (c *grpcPluginClient) GetManifest(ctx context.Context) (*Manifest, error) {
	resp, err := c.client.GetManifest(ctx, &pb.Empty{})
	if err != nil {
		return nil, err
	}
	return &Manifest{
		Name:         resp.Name,
		Version:      resp.Version,
		Description:  resp.Description,
		Author:       resp.Author,
		Schedule:     resp.Schedule,
		Capabilities: resp.Capabilities,
		ConfigSchema: resp.ConfigSchema,
	}, nil
}

func (c *grpcPluginClient) Configure(ctx context.Context, host HostClient, config map[string]string, stateDir string) error {
	// Start a gRPC server for the host client that the plugin can call back to
	hostServer := &grpcHostServer{host: host}

	var brokerID uint32
	if host != nil {
		brokerID = c.broker.NextId()
		go c.broker.AcceptAndServe(brokerID, func(opts []grpc.ServerOption) *grpc.Server {
			s := grpc.NewServer(opts...)
			pb.RegisterReorgHostServer(s, hostServer)
			return s
		})
	}

	resp, err := c.client.Configure(ctx, &pb.ConfigureRequest{
		Config:         config,
		StateDir:       stateDir,
		HostServerPort: brokerID,
	})
	if err != nil {
		return err
	}
	if !resp.Success {
		return status.Error(codes.Internal, resp.Error)
	}
	return nil
}

func (c *grpcPluginClient) Execute(ctx context.Context, params *ExecuteParams) (*ExecuteResult, error) {
	resp, err := c.client.Execute(ctx, &pb.ExecuteRequest{
		DryRun: params.DryRun,
		Params: params.Params,
	})
	if err != nil {
		return nil, err
	}

	result := &ExecuteResult{
		Success: resp.Success,
		Error:   resp.Error,
	}

	if resp.Summary != nil {
		result.Summary = &ExecuteSummary{
			ItemsProcessed: int(resp.Summary.ItemsProcessed),
			ItemsImported:  int(resp.Summary.ItemsImported),
			ItemsSkipped:   int(resp.Summary.ItemsSkipped),
			ItemsFailed:    int(resp.Summary.ItemsFailed),
			Message:        resp.Summary.Message,
		}
	}

	for _, item := range resp.Results {
		result.Results = append(result.Results, ExecuteItem{
			ID:       item.Id,
			Name:     item.Name,
			Action:   item.Action,
			Message:  item.Message,
			Metadata: item.Metadata,
		})
	}

	return result, nil
}

func (c *grpcPluginClient) Shutdown(ctx context.Context) error {
	_, err := c.client.Shutdown(ctx, &pb.Empty{})
	return err
}

// grpcHostServer implements the gRPC server for the host.
// This runs in the host process and serves plugin callbacks.
type grpcHostServer struct {
	pb.UnimplementedReorgHostServer
	host HostClient
}

func (s *grpcHostServer) ListAreas(ctx context.Context, _ *pb.ListAreasRequest) (*pb.ListAreasResponse, error) {
	areas, err := s.host.ListAreas(ctx)
	if err != nil {
		return nil, err
	}
	resp := &pb.ListAreasResponse{}
	for _, a := range areas {
		resp.Areas = append(resp.Areas, areaToPB(a))
	}
	return resp, nil
}

func (s *grpcHostServer) GetArea(ctx context.Context, req *pb.GetAreaRequest) (*pb.GetAreaResponse, error) {
	area, err := s.host.GetArea(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return &pb.GetAreaResponse{Area: areaToPB(area)}, nil
}

func (s *grpcHostServer) CreateArea(ctx context.Context, req *pb.CreateAreaRequest) (*pb.CreateAreaResponse, error) {
	area, err := s.host.CreateArea(ctx, req.Title, req.Content, req.Tags)
	if err != nil {
		return nil, err
	}
	return &pb.CreateAreaResponse{Area: areaToPB(area)}, nil
}

func (s *grpcHostServer) ListProjects(ctx context.Context, req *pb.ListProjectsRequest) (*pb.ListProjectsResponse, error) {
	projects, err := s.host.ListProjects(ctx, req.AreaId)
	if err != nil {
		return nil, err
	}
	resp := &pb.ListProjectsResponse{}
	for _, p := range projects {
		resp.Projects = append(resp.Projects, projectToPB(p))
	}
	return resp, nil
}

func (s *grpcHostServer) ListAllProjects(ctx context.Context, _ *pb.Empty) (*pb.ListAllProjectsResponse, error) {
	projects, err := s.host.ListAllProjects(ctx)
	if err != nil {
		return nil, err
	}
	resp := &pb.ListAllProjectsResponse{}
	for _, p := range projects {
		resp.Projects = append(resp.Projects, projectToPB(p))
	}
	return resp, nil
}

func (s *grpcHostServer) GetProject(ctx context.Context, req *pb.GetProjectRequest) (*pb.GetProjectResponse, error) {
	project, err := s.host.GetProject(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return &pb.GetProjectResponse{Project: projectToPB(project)}, nil
}

func (s *grpcHostServer) CreateProject(ctx context.Context, req *pb.CreateProjectRequest) (*pb.CreateProjectResponse, error) {
	project, err := s.host.CreateProject(ctx, req.Title, req.AreaId, req.Content, req.Tags)
	if err != nil {
		return nil, err
	}
	return &pb.CreateProjectResponse{Project: projectToPB(project)}, nil
}

func (s *grpcHostServer) ListTasks(ctx context.Context, req *pb.ListTasksRequest) (*pb.ListTasksResponse, error) {
	tasks, err := s.host.ListTasks(ctx, req.ProjectId)
	if err != nil {
		return nil, err
	}
	resp := &pb.ListTasksResponse{}
	for _, t := range tasks {
		resp.Tasks = append(resp.Tasks, taskToPB(t))
	}
	return resp, nil
}

func (s *grpcHostServer) CreateTask(ctx context.Context, req *pb.CreateTaskRequest) (*pb.CreateTaskResponse, error) {
	task, err := s.host.CreateTask(ctx, req.Title, req.ProjectId, req.AreaId, req.Content, pbToPriority(req.Priority), req.Tags)
	if err != nil {
		return nil, err
	}
	return &pb.CreateTaskResponse{Task: taskToPB(task)}, nil
}

func (s *grpcHostServer) CategorizeWithContext(ctx context.Context, req *pb.CategorizeRequest) (*pb.CategorizeResponse, error) {
	var projects []ProjectContext
	for _, p := range req.ExistingProjects {
		projects = append(projects, ProjectContext{
			ID:    p.Id,
			Title: p.Title,
			Area:  p.Area,
		})
	}

	result, err := s.host.CategorizeWithContext(ctx, req.Content, projects)
	if err != nil {
		return nil, err
	}

	return &pb.CategorizeResponse{
		Area:              result.Area,
		AreaConfidence:    result.AreaConfidence,
		ProjectId:         result.ProjectID,
		ProjectSuggestion: result.ProjectSuggestion,
		Tags:              result.Tags,
		Summary:           result.Summary,
		IsActionable:      result.IsActionable,
	}, nil
}

func (s *grpcHostServer) ExtractTasks(ctx context.Context, req *pb.ExtractTasksRequest) (*pb.ExtractTasksResponse, error) {
	tasks, err := s.host.ExtractTasks(ctx, req.Content)
	if err != nil {
		return nil, err
	}

	resp := &pb.ExtractTasksResponse{}
	for _, t := range tasks {
		resp.Tasks = append(resp.Tasks, &pb.ExtractedTask{
			Title:       t.Title,
			Description: t.Description,
			Priority:    t.Priority,
			DueDate:     t.DueDate,
			Tags:        t.Tags,
		})
	}
	return resp, nil
}

func (s *grpcHostServer) GetState(ctx context.Context, req *pb.GetStateRequest) (*pb.GetStateResponse, error) {
	value, found, err := s.host.GetState(ctx, req.Key)
	if err != nil {
		return nil, err
	}
	return &pb.GetStateResponse{
		Value: value,
		Found: found,
	}, nil
}

func (s *grpcHostServer) SetState(ctx context.Context, req *pb.SetStateRequest) (*pb.SetStateResponse, error) {
	err := s.host.SetState(ctx, req.Key, req.Value)
	if err != nil {
		return nil, err
	}
	return &pb.SetStateResponse{Success: true}, nil
}

func (s *grpcHostServer) DeleteState(ctx context.Context, req *pb.DeleteStateRequest) (*pb.DeleteStateResponse, error) {
	err := s.host.DeleteState(ctx, req.Key)
	if err != nil {
		return nil, err
	}
	return &pb.DeleteStateResponse{Success: true}, nil
}

// grpcHostClient implements HostClient by calling the host's gRPC server.
// This runs in the plugin process.
type grpcHostClient struct {
	client pb.ReorgHostClient
}

func (c *grpcHostClient) ListAreas(ctx context.Context) ([]*Area, error) {
	resp, err := c.client.ListAreas(ctx, &pb.ListAreasRequest{})
	if err != nil {
		return nil, err
	}
	var areas []*Area
	for _, a := range resp.Areas {
		areas = append(areas, pbToArea(a))
	}
	return areas, nil
}

func (c *grpcHostClient) GetArea(ctx context.Context, id string) (*Area, error) {
	resp, err := c.client.GetArea(ctx, &pb.GetAreaRequest{Id: id})
	if err != nil {
		return nil, err
	}
	return pbToArea(resp.Area), nil
}

func (c *grpcHostClient) CreateArea(ctx context.Context, title, content string, tags []string) (*Area, error) {
	resp, err := c.client.CreateArea(ctx, &pb.CreateAreaRequest{
		Title:   title,
		Content: content,
		Tags:    tags,
	})
	if err != nil {
		return nil, err
	}
	return pbToArea(resp.Area), nil
}

func (c *grpcHostClient) ListProjects(ctx context.Context, areaID string) ([]*Project, error) {
	resp, err := c.client.ListProjects(ctx, &pb.ListProjectsRequest{AreaId: areaID})
	if err != nil {
		return nil, err
	}
	var projects []*Project
	for _, p := range resp.Projects {
		projects = append(projects, pbToProject(p))
	}
	return projects, nil
}

func (c *grpcHostClient) ListAllProjects(ctx context.Context) ([]*Project, error) {
	resp, err := c.client.ListAllProjects(ctx, &pb.Empty{})
	if err != nil {
		return nil, err
	}
	var projects []*Project
	for _, p := range resp.Projects {
		projects = append(projects, pbToProject(p))
	}
	return projects, nil
}

func (c *grpcHostClient) GetProject(ctx context.Context, id string) (*Project, error) {
	resp, err := c.client.GetProject(ctx, &pb.GetProjectRequest{Id: id})
	if err != nil {
		return nil, err
	}
	return pbToProject(resp.Project), nil
}

func (c *grpcHostClient) CreateProject(ctx context.Context, title, areaID, content string, tags []string) (*Project, error) {
	resp, err := c.client.CreateProject(ctx, &pb.CreateProjectRequest{
		Title:   title,
		AreaId:  areaID,
		Content: content,
		Tags:    tags,
	})
	if err != nil {
		return nil, err
	}
	return pbToProject(resp.Project), nil
}

func (c *grpcHostClient) ListTasks(ctx context.Context, projectID string) ([]*Task, error) {
	resp, err := c.client.ListTasks(ctx, &pb.ListTasksRequest{ProjectId: projectID})
	if err != nil {
		return nil, err
	}
	var tasks []*Task
	for _, t := range resp.Tasks {
		tasks = append(tasks, pbToTask(t))
	}
	return tasks, nil
}

func (c *grpcHostClient) CreateTask(ctx context.Context, title, projectID, areaID, content string, priority Priority, tags []string) (*Task, error) {
	resp, err := c.client.CreateTask(ctx, &pb.CreateTaskRequest{
		Title:     title,
		ProjectId: projectID,
		AreaId:    areaID,
		Content:   content,
		Priority:  priorityToPB(priority),
		Tags:      tags,
	})
	if err != nil {
		return nil, err
	}
	return pbToTask(resp.Task), nil
}

func (c *grpcHostClient) CategorizeWithContext(ctx context.Context, content string, existingProjects []ProjectContext) (*CategorizeResult, error) {
	var pbProjects []*pb.ProjectContext
	for _, p := range existingProjects {
		pbProjects = append(pbProjects, &pb.ProjectContext{
			Id:    p.ID,
			Title: p.Title,
			Area:  p.Area,
		})
	}

	resp, err := c.client.CategorizeWithContext(ctx, &pb.CategorizeRequest{
		Content:          content,
		ExistingProjects: pbProjects,
	})
	if err != nil {
		return nil, err
	}

	return &CategorizeResult{
		Area:              resp.Area,
		AreaConfidence:    resp.AreaConfidence,
		ProjectID:         resp.ProjectId,
		ProjectSuggestion: resp.ProjectSuggestion,
		Tags:              resp.Tags,
		Summary:           resp.Summary,
		IsActionable:      resp.IsActionable,
	}, nil
}

func (c *grpcHostClient) ExtractTasks(ctx context.Context, content string) ([]ExtractedTask, error) {
	resp, err := c.client.ExtractTasks(ctx, &pb.ExtractTasksRequest{Content: content})
	if err != nil {
		return nil, err
	}

	var tasks []ExtractedTask
	for _, t := range resp.Tasks {
		tasks = append(tasks, ExtractedTask{
			Title:       t.Title,
			Description: t.Description,
			Priority:    t.Priority,
			DueDate:     t.DueDate,
			Tags:        t.Tags,
		})
	}
	return tasks, nil
}

func (c *grpcHostClient) GetState(ctx context.Context, key string) ([]byte, bool, error) {
	resp, err := c.client.GetState(ctx, &pb.GetStateRequest{Key: key})
	if err != nil {
		return nil, false, err
	}
	return resp.Value, resp.Found, nil
}

func (c *grpcHostClient) SetState(ctx context.Context, key string, value []byte) error {
	_, err := c.client.SetState(ctx, &pb.SetStateRequest{Key: key, Value: value})
	return err
}

func (c *grpcHostClient) DeleteState(ctx context.Context, key string) error {
	_, err := c.client.DeleteState(ctx, &pb.DeleteStateRequest{Key: key})
	return err
}

// FindOrCreateArea finds an area by name or creates it.
func (c *grpcHostClient) FindOrCreateArea(ctx context.Context, name string) (*Area, error) {
	areas, err := c.ListAreas(ctx)
	if err != nil {
		return nil, err
	}

	slug := slugify(name)
	for _, a := range areas {
		if stringsEqualFold(slugify(a.Title), slug) || stringsEqualFold(a.Title, name) {
			return a, nil
		}
	}

	// Create new area
	return c.CreateArea(ctx, name, "", nil)
}

// FindOrCreateProject finds a project by name in an area or creates it.
func (c *grpcHostClient) FindOrCreateProject(ctx context.Context, name, areaID, content string, tags []string) (*Project, error) {
	projects, err := c.ListProjects(ctx, areaID)
	if err != nil {
		return nil, err
	}

	slug := slugify(name)
	for _, p := range projects {
		if stringsEqualFold(slugify(p.Title), slug) || stringsEqualFold(p.Title, name) {
			return p, nil
		}
	}

	// Create new project
	return c.CreateProject(ctx, name, areaID, content, tags)
}

// BuildProjectContext builds a list of existing projects for AI matching.
func (c *grpcHostClient) BuildProjectContext(ctx context.Context) ([]ProjectContext, error) {
	var result []ProjectContext

	areas, err := c.ListAreas(ctx)
	if err != nil {
		return result, err
	}

	for _, area := range areas {
		projects, err := c.ListProjects(ctx, area.ID)
		if err != nil {
			continue
		}
		for _, p := range projects {
			result = append(result, ProjectContext{
				ID:    p.ID,
				Title: p.Title,
				Area:  area.Title,
			})
		}
	}

	return result, nil
}

// Helper functions for string operations

func slugify(s string) string {
	slug := stringsToLower(s)
	slug = stringsReplace(slug, " ", "-")
	var result []rune
	for _, r := range slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result = append(result, r)
		}
	}
	return string(result)
}

func stringsEqualFold(a, b string) bool {
	return stringsToLower(a) == stringsToLower(b)
}

func stringsToLower(s string) string {
	result := make([]rune, len(s))
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			result[i] = r + ('a' - 'A')
		} else {
			result[i] = r
		}
	}
	return string(result)
}

func stringsReplace(s, old, new string) string {
	// Simple string replacement
	var result []rune
	oldRunes := []rune(old)
	newRunes := []rune(new)
	sRunes := []rune(s)

	for i := 0; i < len(sRunes); i++ {
		if i+len(oldRunes) <= len(sRunes) {
			match := true
			for j := 0; j < len(oldRunes); j++ {
				if sRunes[i+j] != oldRunes[j] {
					match = false
					break
				}
			}
			if match {
				result = append(result, newRunes...)
				i += len(oldRunes) - 1
				continue
			}
		}
		result = append(result, sRunes[i])
	}
	return string(result)
}

// Helper functions for type conversions

func areaToPB(a *Area) *pb.Area {
	if a == nil {
		return nil
	}
	return &pb.Area{
		Id:        a.ID,
		Title:     a.Title,
		Content:   a.Content,
		Tags:      a.Tags,
		CreatedAt: timestamppb.New(a.CreatedAt),
		UpdatedAt: timestamppb.New(a.UpdatedAt),
	}
}

func pbToArea(a *pb.Area) *Area {
	if a == nil {
		return nil
	}
	return &Area{
		ID:        a.Id,
		Title:     a.Title,
		Content:   a.Content,
		Tags:      a.Tags,
		CreatedAt: a.CreatedAt.AsTime(),
		UpdatedAt: a.UpdatedAt.AsTime(),
	}
}

func projectToPB(p *Project) *pb.Project {
	if p == nil {
		return nil
	}
	proj := &pb.Project{
		Id:        p.ID,
		Title:     p.Title,
		AreaId:    p.AreaID,
		Content:   p.Content,
		Status:    projectStatusToPB(p.Status),
		Tags:      p.Tags,
		CreatedAt: timestamppb.New(p.CreatedAt),
		UpdatedAt: timestamppb.New(p.UpdatedAt),
	}
	if p.DueDate != nil {
		proj.DueDate = timestamppb.New(*p.DueDate)
	}
	return proj
}

func pbToProject(p *pb.Project) *Project {
	if p == nil {
		return nil
	}
	proj := &Project{
		ID:        p.Id,
		Title:     p.Title,
		AreaID:    p.AreaId,
		Content:   p.Content,
		Status:    pbToProjectStatus(p.Status),
		Tags:      p.Tags,
		CreatedAt: p.CreatedAt.AsTime(),
		UpdatedAt: p.UpdatedAt.AsTime(),
	}
	if p.DueDate != nil {
		t := p.DueDate.AsTime()
		proj.DueDate = &t
	}
	return proj
}

func taskToPB(t *Task) *pb.Task {
	if t == nil {
		return nil
	}
	task := &pb.Task{
		Id:        t.ID,
		Title:     t.Title,
		ProjectId: t.ProjectID,
		AreaId:    t.AreaID,
		Content:   t.Content,
		Status:    taskStatusToPB(t.Status),
		Priority:  priorityToPB(t.Priority),
		Tags:      t.Tags,
		CreatedAt: timestamppb.New(t.CreatedAt),
		UpdatedAt: timestamppb.New(t.UpdatedAt),
	}
	if t.DueDate != nil {
		task.DueDate = timestamppb.New(*t.DueDate)
	}
	return task
}

func pbToTask(t *pb.Task) *Task {
	if t == nil {
		return nil
	}
	task := &Task{
		ID:        t.Id,
		Title:     t.Title,
		ProjectID: t.ProjectId,
		AreaID:    t.AreaId,
		Content:   t.Content,
		Status:    pbToTaskStatus(t.Status),
		Priority:  pbToPriority(t.Priority),
		Tags:      t.Tags,
		CreatedAt: t.CreatedAt.AsTime(),
		UpdatedAt: t.UpdatedAt.AsTime(),
	}
	if t.DueDate != nil {
		tm := t.DueDate.AsTime()
		task.DueDate = &tm
	}
	return task
}

func projectStatusToPB(s ProjectStatus) pb.ProjectStatus {
	switch s {
	case ProjectStatusActive:
		return pb.ProjectStatus_PROJECT_STATUS_ACTIVE
	case ProjectStatusOnHold:
		return pb.ProjectStatus_PROJECT_STATUS_ON_HOLD
	case ProjectStatusCompleted:
		return pb.ProjectStatus_PROJECT_STATUS_COMPLETED
	case ProjectStatusArchived:
		return pb.ProjectStatus_PROJECT_STATUS_ARCHIVED
	default:
		return pb.ProjectStatus_PROJECT_STATUS_UNSPECIFIED
	}
}

func pbToProjectStatus(s pb.ProjectStatus) ProjectStatus {
	switch s {
	case pb.ProjectStatus_PROJECT_STATUS_ACTIVE:
		return ProjectStatusActive
	case pb.ProjectStatus_PROJECT_STATUS_ON_HOLD:
		return ProjectStatusOnHold
	case pb.ProjectStatus_PROJECT_STATUS_COMPLETED:
		return ProjectStatusCompleted
	case pb.ProjectStatus_PROJECT_STATUS_ARCHIVED:
		return ProjectStatusArchived
	default:
		return ProjectStatusActive
	}
}

func taskStatusToPB(s TaskStatus) pb.TaskStatus {
	switch s {
	case TaskStatusPending:
		return pb.TaskStatus_TASK_STATUS_TODO
	case TaskStatusInProgress:
		return pb.TaskStatus_TASK_STATUS_IN_PROGRESS
	case TaskStatusCompleted:
		return pb.TaskStatus_TASK_STATUS_DONE
	case TaskStatusBlocked:
		return pb.TaskStatus_TASK_STATUS_BLOCKED
	case TaskStatusCancelled:
		return pb.TaskStatus_TASK_STATUS_CANCELLED
	default:
		return pb.TaskStatus_TASK_STATUS_UNSPECIFIED
	}
}

func pbToTaskStatus(s pb.TaskStatus) TaskStatus {
	switch s {
	case pb.TaskStatus_TASK_STATUS_TODO:
		return TaskStatusPending
	case pb.TaskStatus_TASK_STATUS_IN_PROGRESS:
		return TaskStatusInProgress
	case pb.TaskStatus_TASK_STATUS_DONE:
		return TaskStatusCompleted
	case pb.TaskStatus_TASK_STATUS_BLOCKED:
		return TaskStatusBlocked
	case pb.TaskStatus_TASK_STATUS_CANCELLED:
		return TaskStatusCancelled
	default:
		return TaskStatusPending
	}
}

func priorityToPB(p Priority) pb.Priority {
	switch p {
	case PriorityLow:
		return pb.Priority_PRIORITY_LOW
	case PriorityMedium:
		return pb.Priority_PRIORITY_MEDIUM
	case PriorityHigh:
		return pb.Priority_PRIORITY_HIGH
	case PriorityUrgent:
		return pb.Priority_PRIORITY_URGENT
	default:
		return pb.Priority_PRIORITY_UNSPECIFIED
	}
}

func pbToPriority(p pb.Priority) Priority {
	switch p {
	case pb.Priority_PRIORITY_LOW:
		return PriorityLow
	case pb.Priority_PRIORITY_MEDIUM:
		return PriorityMedium
	case pb.Priority_PRIORITY_HIGH:
		return PriorityHigh
	case pb.Priority_PRIORITY_URGENT:
		return PriorityUrgent
	default:
		return PriorityMedium
	}
}

// StateJSON is a helper for plugins to store/retrieve JSON state.
type StateJSON struct {
	host HostClient
}

// NewStateJSON creates a new StateJSON helper.
func NewStateJSON(host HostClient) *StateJSON {
	return &StateJSON{host: host}
}

// Get retrieves and unmarshals JSON state.
func (s *StateJSON) Get(ctx context.Context, key string, v interface{}) (bool, error) {
	data, found, err := s.host.GetState(ctx, key)
	if err != nil || !found {
		return found, err
	}
	return true, json.Unmarshal(data, v)
}

// Set marshals and stores JSON state.
func (s *StateJSON) Set(ctx context.Context, key string, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.host.SetState(ctx, key, data)
}

// Ensure interfaces are satisfied
var (
	_ Plugin     = (*grpcPluginClient)(nil)
	_ HostClient = (*grpcHostClient)(nil)
)

// Unused but needed for type conversion helper functions
var _ = time.Now
