package adapter

import (
	"fmt"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	"github.com/fundament-oss/fundament/organization-api/pkg/models"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func ToProjectCreate(req *organizationv1.CreateProjectRequest) models.ProjectCreate {
	return models.ProjectCreate{
		Name: req.Name,
	}
}

func ToProjectUpdate(req *organizationv1.UpdateProjectRequest) (models.ProjectUpdate, error) {
	projectID, err := uuid.Parse(req.ProjectId)
	if err != nil {
		return models.ProjectUpdate{}, fmt.Errorf("project id parse: %w", err)
	}

	return models.ProjectUpdate{
		ProjectID: projectID,
		Name:      req.Name,
	}, nil
}

func FromProjects(projects []db.TenantProject) []*organizationv1.Project {
	result := make([]*organizationv1.Project, 0, len(projects))
	for i := range projects {
		result = append(result, FromProject(&projects[i]))
	}
	return result
}

func FromProject(p *db.TenantProject) *organizationv1.Project {
	return &organizationv1.Project{
		Id:   p.ID.String(),
		Name: p.Name,
		CreatedAt: timestamppb.New(p.Created.Time),
	}
}

func FromProjectNamespaces(namespaces []db.TenantNamespace) []*organizationv1.ProjectNamespace {
	result := make([]*organizationv1.ProjectNamespace, 0, len(namespaces))
	for i := range namespaces {
		result = append(result, FromProjectNamespace(&namespaces[i]))
	}
	return result
}

func FromProjectNamespace(ns *db.TenantNamespace) *organizationv1.ProjectNamespace {
	return &organizationv1.ProjectNamespace{
		Id:        ns.ID.String(),
		Name:      ns.Name,
		ClusterId: ns.ClusterID.String(),
		CreatedAt: timestamppb.New(ns.Created.Time),
	}
}
