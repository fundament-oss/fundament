package dcim

import (
	"context"

	"connectrpc.com/connect"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (s *Server) ListTaskSteps(_ context.Context, _ *connect.Request[dcimv1.ListTaskStepsRequest]) (*connect.Response[dcimv1.ListTaskStepsResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) CreateTaskStep(_ context.Context, _ *connect.Request[dcimv1.CreateTaskStepRequest]) (*connect.Response[dcimv1.CreateTaskStepResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) UpdateTaskStep(_ context.Context, _ *connect.Request[dcimv1.UpdateTaskStepRequest]) (*connect.Response[emptypb.Empty], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) DeleteTaskStep(_ context.Context, _ *connect.Request[dcimv1.DeleteTaskStepRequest]) (*connect.Response[emptypb.Empty], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}
