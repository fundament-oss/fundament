package authz

import (
	"testing"

	"github.com/google/uuid"
)

func TestObjectConstructors(t *testing.T) {
	id := uuid.New()

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

	gotUser := req.Subject.String()
	gotObject := req.Resource.String()
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
	userID := uuid.New()
	projectID := uuid.New()
	organizationID := uuid.New()

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
					Subject:  Organization(organizationID), // Override subject
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

func TestEvaluations_InvalidSemantic(t *testing.T) {
	// Create a client with an invalid configuration to test error handling.
	// We can't actually call the OpenFGA server, but we can test the semantic validation.
	client := &Client{fga: nil}

	userID := uuid.New()
	projectID := uuid.New()
	subject := User(userID)

	req := EvaluationsRequest{
		Subject: &subject,
		Evaluations: []EvaluationRequest{
			{
				Resource: Project(projectID),
				Action:   CanView(),
			},
		},
		Options: &EvaluationsOptions{
			Semantic: "invalid_semantic",
		},
	}

	_, err := client.Evaluations(t.Context(), req)
	if err == nil {
		t.Fatal("expected error for invalid semantic, got nil")
	}

	expectedMsg := `unsupported evaluation semantic: "invalid_semantic"`
	if err.Error() != expectedMsg {
		t.Errorf("error = %q, want %q", err.Error(), expectedMsg)
	}
}

func TestEvaluations_ValidSemantics(t *testing.T) {
	// Test that all valid semantics are accepted (validation only, no actual evaluation)
	validSemantics := []EvaluationSemantic{
		ExecuteAll,
		DenyOnFirstDeny,
		PermitOnFirstPermit,
	}

	for _, semantic := range validSemantics {
		t.Run(string(semantic), func(t *testing.T) {
			// Verify the semantic constants have expected values
			switch semantic {
			case ExecuteAll:
				if semantic != "execute_all" {
					t.Errorf("ExecuteAll = %q, want %q", semantic, "execute_all")
				}
			case DenyOnFirstDeny:
				if semantic != "deny_on_first_deny" {
					t.Errorf("DenyOnFirstDeny = %q, want %q", semantic, "deny_on_first_deny")
				}
			case PermitOnFirstPermit:
				if semantic != "permit_on_first_permit" {
					t.Errorf("PermitOnFirstPermit = %q, want %q", semantic, "permit_on_first_permit")
				}
			}
		})
	}
}

func TestNew_InvalidConfig(t *testing.T) {
	// Test that New returns an error for invalid configuration
	cfg := Config{
		APIURL:  "", // Empty URL should cause an error
		StoreID: "",
	}

	_, err := New(cfg)
	if err == nil {
		t.Fatal("expected error for empty API URL, got nil")
	}
}

func TestDecision_DefaultsToFalse(t *testing.T) {
	// Verify that a zero-value Decision defaults to deny (false)
	var d Decision
	if d.Decision != false {
		t.Errorf("zero-value Decision.Decision = %v, want false", d.Decision)
	}
}
