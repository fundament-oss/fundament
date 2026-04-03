package idempotency

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type testProcedure struct {
	resourceType ResourceType
	resourceID   uuid.UUID
	status       string
	statusErr    error
}

func (p *testProcedure) ResourceType() ResourceType {
	return p.resourceType
}

func (p *testProcedure) ResolveStatus(_ context.Context, _ uuid.UUID) (string, error) {
	return p.status, p.statusErr
}

func (p *testProcedure) ExtractResourceID(_ any) (uuid.UUID, error) {
	return p.resourceID, nil
}

func (p *testProcedure) DeserializeResponse(data []byte) (connect.AnyResponse, error) {
	msg := &wrapperspb.StringValue{}
	if err := proto.Unmarshal(data, msg); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	return connect.NewResponse(msg), nil
}

func TestHashEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b []byte
		want bool
	}{
		{"both nil", nil, nil, false},
		{"a nil", nil, []byte{1}, false},
		{"b nil", []byte{1}, nil, false},
		{"equal", []byte{1, 2, 3}, []byte{1, 2, 3}, true},
		{"different", []byte{1, 2, 3}, []byte{1, 2, 4}, false},
		{"different length", []byte{1, 2}, []byte{1, 2, 3}, false},
		{"empty equal", []byte{}, []byte{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hashEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("hashEqual(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestHashRequest(t *testing.T) {
	hash, err := hashRequest(connect.NewRequest(wrapperspb.String("hello")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hash == nil {
		t.Fatal("expected non-nil hash")
	}
	if len(hash) != sha256.Size {
		t.Errorf("expected hash length %d, got %d", sha256.Size, len(hash))
	}

	// Same message should produce same hash.
	hash2, err := hashRequest(connect.NewRequest(wrapperspb.String("hello")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hashEqual(hash, hash2) {
		t.Error("expected equal hashes for same message")
	}

	// Different message should produce different hash.
	hash3, err := hashRequest(connect.NewRequest(wrapperspb.String("world")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hashEqual(hash, hash3) {
		t.Error("expected different hashes for different messages")
	}
}

func TestHandleReplay_ProcedureMismatch(t *testing.T) {
	logger := slog.Default()
	cached := &CachedResponse{
		Procedure:   "/other.Procedure",
		RequestHash: []byte{1, 2, 3},
	}

	proc := &testProcedure{resourceType: ResourceProject}

	_, err := handleReplay(context.Background(), logger, cached, []byte{1, 2, 3}, "/my.Procedure", proc)
	if err == nil {
		t.Fatal("expected error for procedure mismatch")
	}
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatal("expected connect.Error")
	}
	if connectErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", connectErr.Code())
	}
}

func TestHandleReplay_RequestHashMismatch(t *testing.T) {
	logger := slog.Default()
	cached := &CachedResponse{
		Procedure:   "/my.Procedure",
		RequestHash: []byte{1, 2, 3},
	}

	proc := &testProcedure{resourceType: ResourceProject}

	_, err := handleReplay(context.Background(), logger, cached, []byte{4, 5, 6}, "/my.Procedure", proc)
	if err == nil {
		t.Fatal("expected error for hash mismatch")
	}
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatal("expected connect.Error")
	}
	if connectErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", connectErr.Code())
	}
}

func TestHandleReplay_Success(t *testing.T) {
	logger := slog.Default()
	resourceID := uuid.New()

	msg := wrapperspb.String("cached-response")
	responseBytes, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}

	reqHash := []byte{1, 2, 3}
	cached := &CachedResponse{
		Procedure:     "/my.Procedure",
		RequestHash:   reqHash,
		ResponseBytes: responseBytes,
		ResourceID:    resourceID,
	}

	proc := &testProcedure{
		resourceType: ResourceProject,
		resourceID:   resourceID,
		status:       "completed",
	}

	resp, err := handleReplay(context.Background(), logger, cached, reqHash, "/my.Procedure", proc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Header().Get(HeaderIdempotencyStatus) != "completed" {
		t.Errorf("expected status 'completed', got %q", resp.Header().Get(HeaderIdempotencyStatus))
	}

	respMsg, ok := resp.Any().(*wrapperspb.StringValue)
	if !ok {
		t.Fatal("expected *wrapperspb.StringValue")
	}
	if respMsg.GetValue() != "cached-response" {
		t.Errorf("expected 'cached-response', got %q", respMsg.GetValue())
	}
}

func TestHandleReplay_StatusResolverFallback(t *testing.T) {
	logger := slog.Default()
	resourceID := uuid.New()

	msg := wrapperspb.String("test")
	responseBytes, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}

	reqHash := []byte{1, 2, 3}
	cached := &CachedResponse{
		Procedure:     "/my.Procedure",
		RequestHash:   reqHash,
		ResponseBytes: responseBytes,
		ResourceID:    resourceID,
	}

	// ResolveStatus returns an error — should default to "processing".
	proc := &testProcedure{
		resourceType: ResourceProject,
		resourceID:   resourceID,
		statusErr:    errors.New("resolver failed"),
	}

	resp, err := handleReplay(context.Background(), logger, cached, reqHash, "/my.Procedure", proc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Header().Get(HeaderIdempotencyStatus) != "processing" {
		t.Errorf("expected status 'processing', got %q", resp.Header().Get(HeaderIdempotencyStatus))
	}
}

func TestNewInterceptor_NilStore(t *testing.T) {
	logger := slog.Default()
	interceptorFunc := NewInterceptor(logger, nil, nil, nil)

	called := false
	next := func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		called = true
		return connect.NewResponse(wrapperspb.String("ok")), nil
	}

	handler := interceptorFunc(next)
	req := connect.NewRequest(wrapperspb.String("test"))
	req.Header().Set(HeaderIdempotencyKey, uuid.New().String())

	_, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected next handler to be called when store is nil")
	}
}
