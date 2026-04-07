package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/proto"

	"github.com/fundament-oss/fundament/common/idempotency"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func buildProcedures(queries *db.Queries) map[string]idempotency.Procedure {
	return map[string]idempotency.Procedure{
		"/organization.v1.ProjectService/CreateProject": &idempotency.ProcedureFunc{
			Type: idempotency.ResourceProject,
			ResolveStatusFn: outboxResolver(func(ctx context.Context, id pgtype.UUID) (string, error) {
				return queries.OutboxStatusByProjectID(ctx, db.OutboxStatusByProjectIDParams{ProjectID: id})
			}),
			ExtractIDFn: extractID(func(resp any) string {
				return resp.(*organizationv1.CreateProjectResponse).GetProjectId()
			}),
			DeserializeFn: deserializeProto(func() *organizationv1.CreateProjectResponse {
				return &organizationv1.CreateProjectResponse{}
			}),
		},
		"/organization.v1.ProjectService/AddProjectMember": &idempotency.ProcedureFunc{
			Type: idempotency.ResourceProjectMember,
			ResolveStatusFn: outboxResolver(func(ctx context.Context, id pgtype.UUID) (string, error) {
				return queries.OutboxStatusByProjectMemberID(ctx, db.OutboxStatusByProjectMemberIDParams{ProjectMemberID: id})
			}),
			ExtractIDFn: extractID(func(resp any) string {
				return resp.(*organizationv1.AddProjectMemberResponse).GetMemberId()
			}),
			DeserializeFn: deserializeProto(func() *organizationv1.AddProjectMemberResponse {
				return &organizationv1.AddProjectMemberResponse{}
			}),
		},
		"/organization.v1.ClusterService/CreateCluster": &idempotency.ProcedureFunc{
			Type: idempotency.ResourceCluster,
			ResolveStatusFn: outboxResolver(func(ctx context.Context, id pgtype.UUID) (string, error) {
				return queries.OutboxStatusByClusterID(ctx, db.OutboxStatusByClusterIDParams{ClusterID: id})
			}),
			ExtractIDFn: extractID(func(resp any) string {
				return resp.(*organizationv1.CreateClusterResponse).GetClusterId()
			}),
			DeserializeFn: deserializeProto(func() *organizationv1.CreateClusterResponse {
				return &organizationv1.CreateClusterResponse{}
			}),
		},
		"/organization.v1.ClusterService/CreateNodePool": &idempotency.ProcedureFunc{
			Type: idempotency.ResourceNodePool,
			ResolveStatusFn: outboxResolver(func(ctx context.Context, id pgtype.UUID) (string, error) {
				return queries.OutboxStatusByNodePoolID(ctx, db.OutboxStatusByNodePoolIDParams{NodePoolID: id})
			}),
			ExtractIDFn: extractID(func(resp any) string {
				return resp.(*organizationv1.CreateNodePoolResponse).GetNodePoolId()
			}),
			DeserializeFn: deserializeProto(func() *organizationv1.CreateNodePoolResponse {
				return &organizationv1.CreateNodePoolResponse{}
			}),
		},
		"/organization.v1.ClusterService/AddInstall": &idempotency.ProcedureFunc{
			Type: idempotency.ResourceInstall,
			ResolveStatusFn: outboxResolver(func(ctx context.Context, id pgtype.UUID) (string, error) {
				return queries.OutboxStatusByInstallID(ctx, db.OutboxStatusByInstallIDParams{InstallID: id})
			}),
			ExtractIDFn: extractID(func(resp any) string {
				return resp.(*organizationv1.AddInstallResponse).GetInstallId()
			}),
			DeserializeFn: deserializeProto(func() *organizationv1.AddInstallResponse {
				return &organizationv1.AddInstallResponse{}
			}),
		},
		"/organization.v1.NamespaceService/CreateNamespace": &idempotency.ProcedureFunc{
			Type: idempotency.ResourceNamespace,
			ResolveStatusFn: outboxResolver(func(ctx context.Context, id pgtype.UUID) (string, error) {
				return queries.OutboxStatusByNamespaceID(ctx, db.OutboxStatusByNamespaceIDParams{NamespaceID: id})
			}),
			ExtractIDFn: extractID(func(resp any) string {
				return resp.(*organizationv1.CreateNamespaceResponse).GetNamespaceId()
			}),
			DeserializeFn: deserializeProto(func() *organizationv1.CreateNamespaceResponse {
				return &organizationv1.CreateNamespaceResponse{}
			}),
		},
		"/organization.v1.APIKeyService/CreateAPIKey": &idempotency.ProcedureFunc{
			Type: idempotency.ResourceAPIKey,
			ResolveStatusFn: outboxResolver(func(ctx context.Context, id pgtype.UUID) (string, error) {
				return queries.OutboxStatusByApiKeyID(ctx, db.OutboxStatusByApiKeyIDParams{ApiKeyID: id})
			}),
			ExtractIDFn: extractID(func(resp any) string {
				return resp.(*organizationv1.CreateAPIKeyResponse).GetId()
			}),
			DeserializeFn: deserializeProto(func() *organizationv1.CreateAPIKeyResponse {
				return &organizationv1.CreateAPIKeyResponse{}
			}),
		},
		"/organization.v1.InviteService/InviteMember": &idempotency.ProcedureFunc{
			Type: idempotency.ResourceOrganizationUser,
			ResolveStatusFn: outboxResolver(func(ctx context.Context, id pgtype.UUID) (string, error) {
				return queries.OutboxStatusByOrganizationUserID(ctx, db.OutboxStatusByOrganizationUserIDParams{OrganizationUserID: id})
			}),
			ExtractIDFn: extractID(func(resp any) string {
				return resp.(*organizationv1.InviteMemberResponse).GetInvitationId()
			}),
			DeserializeFn: deserializeProto(func() *organizationv1.InviteMemberResponse {
				return &organizationv1.InviteMemberResponse{}
			}),
		},
	}
}

// outboxResolver adapts an outbox query function into a ResolveStatusFn.
func outboxResolver(
	query func(ctx context.Context, id pgtype.UUID) (string, error),
) func(context.Context, uuid.UUID) (string, error) {
	return func(ctx context.Context, resourceID uuid.UUID) (string, error) {
		status, err := query(ctx, pgtype.UUID{Bytes: resourceID, Valid: true})
		if err != nil {
			return "", fmt.Errorf("resolve outbox status: %w", err)
		}
		return status, nil
	}
}

// extractID adapts a response field getter into an ExtractIDFn.
func extractID(
	getter func(resp any) string,
) func(any) (uuid.UUID, error) {
	return func(resp any) (uuid.UUID, error) {
		id, err := uuid.Parse(getter(resp))
		if err != nil {
			return uuid.Nil, fmt.Errorf("parse resource ID: %w", err)
		}
		return id, nil
	}
}

// deserializeProto creates a DeserializeFn for a protobuf response message.
func deserializeProto[T interface {
	*E
	proto.Message
}, E any](newMsg func() T) func([]byte) (connect.AnyResponse, error) {
	return func(data []byte) (connect.AnyResponse, error) {
		msg := newMsg()
		if err := proto.Unmarshal(data, msg); err != nil {
			return nil, fmt.Errorf("unmarshal response: %w", err)
		}
		return connect.NewResponse(msg), nil
	}
}
