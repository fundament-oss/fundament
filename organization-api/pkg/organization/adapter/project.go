package adapter

import (
	"fmt"
	"time"

	"github.com/google/uuid"

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
		CreatedAt: &organizationv1.Timestamp{
			Value: p.Created.Time.Format(time.RFC3339),
		},
	}
}

func FromProjectNamespaces(rows []db.NamespaceProjectListByProjectIDRow) []*organizationv1.ProjectNamespace {
	result := make([]*organizationv1.ProjectNamespace, 0, len(rows))
	for i := range rows {
		result = append(result, FromProjectNamespace(&rows[i]))
	}
	return result
}

func FromProjectNamespace(row *db.NamespaceProjectListByProjectIDRow) *organizationv1.ProjectNamespace {
	return &organizationv1.ProjectNamespace{
		NamespaceId: row.NamespaceID.String(),
		AttachedAt: &organizationv1.Timestamp{
			Value: row.Created.Time.Format(time.RFC3339),
		},
	}
}
