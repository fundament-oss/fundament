package dcim

import (
	"context"

	"connectrpc.com/connect"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (s *Server) ListNotes(ctx context.Context, req *connect.Request[dcimv1.ListNotesRequest]) (*connect.Response[dcimv1.ListNotesResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) CreateNote(ctx context.Context, req *connect.Request[dcimv1.CreateNoteRequest]) (*connect.Response[dcimv1.CreateNoteResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) DeleteNote(ctx context.Context, req *connect.Request[dcimv1.DeleteNoteRequest]) (*connect.Response[emptypb.Empty], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}
