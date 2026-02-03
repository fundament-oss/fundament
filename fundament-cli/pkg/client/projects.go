package client

import (
	"context"

	"connectrpc.com/connect"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

// ListProjects lists all projects.
func (c *Client) ListProjects(ctx context.Context) ([]*organizationv1.Project, error) {
	resp, err := c.projects().ListProjects(ctx, connect.NewRequest(&organizationv1.ListProjectsRequest{}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.Projects, nil
}

// GetProject gets a project by ID.
func (c *Client) GetProject(ctx context.Context, projectID string) (*organizationv1.Project, error) {
	resp, err := c.projects().GetProject(ctx, connect.NewRequest(&organizationv1.GetProjectRequest{
		ProjectId: projectID,
	}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.Project, nil
}

// CreateProject creates a new project.
func (c *Client) CreateProject(ctx context.Context, name string) (string, error) {
	resp, err := c.projects().CreateProject(ctx, connect.NewRequest(&organizationv1.CreateProjectRequest{
		Name: name,
	}))
	if err != nil {
		return "", err
	}
	return resp.Msg.ProjectId, nil
}
