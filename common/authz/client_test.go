package authz

import (
	"testing"

	"github.com/google/uuid"
)

func TestObjectConstructors(t *testing.T) {
	id := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")

	tests := []struct {
		name     string
		object   Object
		wantType ObjectType
		wantID   string
	}{
		{
			name:     "User",
			object:   User(id),
			wantType: ObjectTypeUser,
			wantID:   id.String(),
		},
		{
			name:     "Organization",
			object:   Organization(id),
			wantType: ObjectTypeOrganization,
			wantID:   id.String(),
		},
		{
			name:     "Project",
			object:   Project(id),
			wantType: ObjectTypeProject,
			wantID:   id.String(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.object.Type != tt.wantType {
				t.Errorf("Type = %v, want %v", tt.object.Type, tt.wantType)
			}
			if tt.object.ID != tt.wantID {
				t.Errorf("ID = %v, want %v", tt.object.ID, tt.wantID)
			}
		})
	}
}

func TestActionConstructors(t *testing.T) {
	tests := []struct {
		name     string
		action   Action
		wantName ActionName
	}{
		{"Member", Member(), ActionMember},
		{"Admin", Admin(), ActionAdmin},
		{"Viewer", Viewer(), ActionViewer},
		{"OrganizationAction", OrganizationAction(), ActionOrganization},
		{"CanView", CanView(), ActionCanView},
		{"CanEdit", CanEdit(), ActionCanEdit},
		{"CanDelete", CanDelete(), ActionCanDelete},
		{"CanManageMembers", CanManageMembers(), ActionCanManageMembers},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.action.Name != tt.wantName {
				t.Errorf("Name = %v, want %v", tt.action.Name, tt.wantName)
			}
		})
	}
}

func TestEvaluationRequest(t *testing.T) {
	userID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	projectID := uuid.MustParse("660e8400-e29b-41d4-a716-446655440000")

	req := EvaluationRequest{
		Subject:  User(userID),
		Resource: Project(projectID),
		Action:   CanView(),
	}

	// Verify the request can be formatted as OpenFGA expects
	wantUser := "user:550e8400-e29b-41d4-a716-446655440000"
	wantObject := "project:660e8400-e29b-41d4-a716-446655440000"
	wantRelation := "can_view"

	gotUser := string(req.Subject.Type) + ":" + req.Subject.ID
	gotObject := string(req.Resource.Type) + ":" + req.Resource.ID
	gotRelation := string(req.Action.Name)

	if gotUser != wantUser {
		t.Errorf("User = %v, want %v", gotUser, wantUser)
	}
	if gotObject != wantObject {
		t.Errorf("Object = %v, want %v", gotObject, wantObject)
	}
	if gotRelation != wantRelation {
		t.Errorf("Relation = %v, want %v", gotRelation, wantRelation)
	}
}

func TestMergeEvaluation(t *testing.T) {
	userID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	projectID := uuid.MustParse("660e8400-e29b-41d4-a716-446655440000")

	subject := User(userID)
	action := CanView()

	t.Run("applies defaults", func(t *testing.T) {
		req := EvaluationsRequest{
			Subject: &subject,
			Action:  &action,
			Evaluations: []EvaluationRequest{
				{Resource: Project(projectID)},
			},
		}

		merged := mergeEvaluation(req.Evaluations[0], req)

		if merged.Subject.Type != ObjectTypeUser {
			t.Errorf("Subject.Type = %v, want %v", merged.Subject.Type, ObjectTypeUser)
		}
		if merged.Action.Name != ActionCanView {
			t.Errorf("Action.Name = %v, want %v", merged.Action.Name, ActionCanView)
		}
		if merged.Resource.Type != ObjectTypeProject {
			t.Errorf("Resource.Type = %v, want %v", merged.Resource.Type, ObjectTypeProject)
		}
	})

	t.Run("evaluation overrides defaults", func(t *testing.T) {
		req := EvaluationsRequest{
			Subject: &subject,
			Action:  &action,
			Evaluations: []EvaluationRequest{
				{
					Subject:  Organization(projectID), // Override subject
					Resource: Project(projectID),
					Action:   CanEdit(), // Override action
				},
			},
		}

		merged := mergeEvaluation(req.Evaluations[0], req)

		if merged.Subject.Type != ObjectTypeOrganization {
			t.Errorf("Subject.Type = %v, want %v", merged.Subject.Type, ObjectTypeOrganization)
		}
		if merged.Action.Name != ActionCanEdit {
			t.Errorf("Action.Name = %v, want %v", merged.Action.Name, ActionCanEdit)
		}
	})

	t.Run("merges context", func(t *testing.T) {
		req := EvaluationsRequest{
			Subject: &subject,
			Action:  &action,
			Context: Context{"default_key": "default_value", "shared": "from_default"},
			Evaluations: []EvaluationRequest{
				{
					Resource: Project(projectID),
					Context:  Context{"eval_key": "eval_value", "shared": "from_eval"},
				},
			},
		}

		merged := mergeEvaluation(req.Evaluations[0], req)

		if merged.Context["default_key"] != "default_value" {
			t.Errorf("Context[default_key] = %v, want default_value", merged.Context["default_key"])
		}
		if merged.Context["eval_key"] != "eval_value" {
			t.Errorf("Context[eval_key] = %v, want eval_value", merged.Context["eval_key"])
		}
		// Evaluation context should override default
		if merged.Context["shared"] != "from_eval" {
			t.Errorf("Context[shared] = %v, want from_eval", merged.Context["shared"])
		}
	})
}

func TestObjectTypeValues(t *testing.T) {
	// Verify the string values match what OpenFGA expects
	tests := []struct {
		objectType ObjectType
		want       string
	}{
		{ObjectTypeUser, "user"},
		{ObjectTypeOrganization, "organization"},
		{ObjectTypeProject, "project"},
	}

	for _, tt := range tests {
		t.Run(string(tt.objectType), func(t *testing.T) {
			if string(tt.objectType) != tt.want {
				t.Errorf("ObjectType = %v, want %v", tt.objectType, tt.want)
			}
		})
	}
}

func TestActionNameValues(t *testing.T) {
	// Verify the string values match what OpenFGA expects
	tests := []struct {
		action ActionName
		want   string
	}{
		{ActionMember, "member"},
		{ActionAdmin, "admin"},
		{ActionViewer, "viewer"},
		{ActionOrganization, "organization"},
		{ActionCanView, "can_view"},
		{ActionCanEdit, "can_edit"},
		{ActionCanDelete, "can_delete"},
		{ActionCanManageMembers, "can_manage_members"},
	}

	for _, tt := range tests {
		t.Run(string(tt.action), func(t *testing.T) {
			if string(tt.action) != tt.want {
				t.Errorf("ActionName = %v, want %v", tt.action, tt.want)
			}
		})
	}
}

func TestTypicalUsage(t *testing.T) {
	userID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	projectID := uuid.MustParse("660e8400-e29b-41d4-a716-446655440000")

	// Typical evaluation: can user view project?
	req := EvaluationRequest{
		Subject:  User(userID),
		Resource: Project(projectID),
		Action:   CanView(),
	}

	// Verify it formats correctly for OpenFGA
	if string(req.Subject.Type)+":"+req.Subject.ID != "user:550e8400-e29b-41d4-a716-446655440000" {
		t.Error("Subject formatted incorrectly")
	}
	if string(req.Resource.Type)+":"+req.Resource.ID != "project:660e8400-e29b-41d4-a716-446655440000" {
		t.Error("Resource formatted incorrectly")
	}
	if string(req.Action.Name) != "can_view" {
		t.Error("Action formatted incorrectly")
	}
}

func TestSameObjectAsSubjectAndResource(t *testing.T) {
	// An organization can be both a subject and resource
	orgID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	org := Organization(orgID)

	// Use same object as both subject and resource
	req := EvaluationRequest{
		Subject:  org,
		Resource: org,
		Action:   Admin(),
	}

	if req.Subject.Type != req.Resource.Type {
		t.Error("Subject and Resource should have same type")
	}
	if req.Subject.ID != req.Resource.ID {
		t.Error("Subject and Resource should have same ID")
	}
}
