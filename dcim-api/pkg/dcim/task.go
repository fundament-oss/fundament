package dcim

import (
	"context"

	"connectrpc.com/connect"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) ListTasks(_ context.Context, _ *connect.Request[dcimv1.ListTasksRequest]) (*connect.Response[dcimv1.ListTasksResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) GetTask(_ context.Context, _ *connect.Request[dcimv1.GetTaskRequest]) (*connect.Response[dcimv1.GetTaskResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) CreateTask(_ context.Context, _ *connect.Request[dcimv1.CreateTaskRequest]) (*connect.Response[dcimv1.CreateTaskResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) UpdateTask(_ context.Context, _ *connect.Request[dcimv1.UpdateTaskRequest]) (*connect.Response[dcimv1.UpdateTaskResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) DeleteTask(_ context.Context, _ *connect.Request[dcimv1.DeleteTaskRequest]) (*connect.Response[dcimv1.DeleteTaskResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}
