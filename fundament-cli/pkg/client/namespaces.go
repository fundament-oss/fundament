package client

import (
	"context"

	"connectrpc.com/connect"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

// ListClusterNamespaces lists all namespaces for a cluster.
func (c *Client) ListClusterNamespaces(ctx context.Context, clusterID string) ([]*organizationv1.ClusterNamespace, error) {
	resp, err := c.clusters().ListClusterNamespaces(ctx, connect.NewRequest(&organizationv1.ListClusterNamespacesRequest{
		ClusterId: clusterID,
	}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.Namespaces, nil
}

// ListProjectNamespaces lists all namespaces for a project.
func (c *Client) ListProjectNamespaces(ctx context.Context, projectID string) ([]*organizationv1.ProjectNamespace, error) {
	resp, err := c.projects().ListProjectNamespaces(ctx, connect.NewRequest(&organizationv1.ListProjectNamespacesRequest{
		ProjectId: projectID,
	}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.Namespaces, nil
}

// CreateNamespace creates a new namespace.
func (c *Client) CreateNamespace(ctx context.Context, projectID, clusterID, name string) (string, error) {
	resp, err := c.clusters().CreateNamespace(ctx, connect.NewRequest(&organizationv1.CreateNamespaceRequest{
		ProjectId: projectID,
		ClusterId: clusterID,
		Name:      name,
	}))
	if err != nil {
		return "", err
	}
	return resp.Msg.NamespaceId, nil
}

// DeleteNamespace deletes a namespace.
func (c *Client) DeleteNamespace(ctx context.Context, namespaceID string) error {
	_, err := c.clusters().DeleteNamespace(ctx, connect.NewRequest(&organizationv1.DeleteNamespaceRequest{
		NamespaceId: namespaceID,
	}))
	return err
}
