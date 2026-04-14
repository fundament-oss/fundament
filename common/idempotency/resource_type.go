package idempotency

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
)

// ResourceType identifies which entity FK column to use in the idempotency_keys table.
type ResourceType int

const (
	ResourceProject ResourceType = iota + 1
	ResourceProjectMember
	ResourceCluster
	ResourceNodePool
	ResourceNamespace
	ResourceAPIKey
	ResourceOrganizationUser
)

func (r ResourceType) String() string {
	switch r {
	case ResourceProject:
		return "project"
	case ResourceProjectMember:
		return "project_member"
	case ResourceCluster:
		return "cluster"
	case ResourceNodePool:
		return "node_pool"
	case ResourceNamespace:
		return "namespace"
	case ResourceAPIKey:
		return "api_key"
	case ResourceOrganizationUser:
		return "organization_user"
	default:
		panic(fmt.Sprintf("unknown resource type: %d", r))
	}
}

// Procedure defines the idempotency behaviour for a single Create procedure.
type Procedure interface {
	ResourceType() ResourceType
	ResolveStatus(ctx context.Context, resourceID uuid.UUID) (string, error)
	ExtractResourceID(resp any) (uuid.UUID, error)
	DeserializeResponse(data []byte) (connect.AnyResponse, error)
}

// ProcedureFunc is a convenience implementation of Procedure using function fields.
type ProcedureFunc struct {
	Type            ResourceType
	ResolveStatusFn func(ctx context.Context, resourceID uuid.UUID) (string, error)
	ExtractIDFn     func(resp any) (uuid.UUID, error)
	DeserializeFn   func(data []byte) (connect.AnyResponse, error)
}

func (p *ProcedureFunc) ResourceType() ResourceType { return p.Type }
func (p *ProcedureFunc) ResolveStatus(ctx context.Context, id uuid.UUID) (string, error) {
	return p.ResolveStatusFn(ctx, id)
}
func (p *ProcedureFunc) ExtractResourceID(resp any) (uuid.UUID, error) {
	return p.ExtractIDFn(resp)
}
func (p *ProcedureFunc) DeserializeResponse(data []byte) (connect.AnyResponse, error) {
	return p.DeserializeFn(data)
}
