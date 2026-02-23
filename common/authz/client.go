package authz

import (
	"context"
	"fmt"
	"maps"

	"github.com/openfga/go-sdk/client"
)

// Config holds configuration for the OpenFGA client.
type Config struct {
	APIURL               string `env:"OPENFGA_API_URL,required,notEmpty"`
	StoreID              string `env:"OPENFGA_STORE_ID,required,notEmpty"`
	AuthorizationModelID string `env:"OPENFGA_AUTHORIZATION_MODEL_ID"`
}

// Client wraps the OpenFGA SDK client with an AuthZEN-compatible interface.
// See https://openid.github.io/authzen/ for the AuthZEN specification.
type Client struct {
	fga *client.OpenFgaClient
}

// New creates a new authorization client.
func New(cfg Config) (*Client, error) {
	fgaClient, err := client.NewSdkClient(&client.ClientConfiguration{
		ApiUrl:               cfg.APIURL,
		StoreId:              cfg.StoreID,
		AuthorizationModelId: cfg.AuthorizationModelID,
	})
	if err != nil {
		return nil, fmt.Errorf("create OpenFGA client: %w", err)
	}

	return &Client{fga: fgaClient}, nil
}

// Evaluate performs a single access evaluation following the AuthZEN Access Evaluation API.
// Returns a Decision indicating whether the subject can perform the action on the resource.
func (c *Client) Evaluate(ctx context.Context, req EvaluationRequest) (Decision, error) {
	// Build OpenFGA check context from the AuthZEN request context and any
	// object/action properties, so that conditions can be evaluated correctly.
	checkContext := map[string]any{}
	if req.Context != nil {
		checkContext = maps.Clone(req.Context)
	}

	// Map resource (object) properties into the context under the "object" key.
	if req.Resource.Properties != nil {
		objProps, ok := checkContext["object"].(map[string]any)
		if !ok || objProps == nil {
			objProps = make(map[string]any)
		}

		maps.Copy(objProps, req.Resource.Properties)
		checkContext["object"] = objProps
	}

	// Map action properties into the context under the "action" key.
	if req.Action.Properties != nil {
		actProps, ok := checkContext["action"].(map[string]any)
		if !ok || actProps == nil {
			actProps = make(map[string]any)
		}

		maps.Copy(actProps, req.Action.Properties)
		checkContext["action"] = actProps
	}

	// Construct the OpenFGA check request, including context if any was provided.
	checkReq := client.ClientCheckRequest{
		User:     string(req.Subject.Type) + ":" + req.Subject.ID,
		Relation: string(req.Action.Name),
		Object:   string(req.Resource.Type) + ":" + req.Resource.ID,
	}

	if len(checkContext) > 0 {
		checkReq.Context = &checkContext
	}

	resp, err := c.fga.Check(ctx).Body(checkReq).Execute()
	if err != nil {
		return Decision{Decision: false}, fmt.Errorf("check: %w", err)
	}

	decision := false
	if resp.Allowed != nil {
		decision = *resp.Allowed
	}

	return Decision{Decision: decision}, nil
}

// Evaluations performs batch access evaluations following the AuthZEN Access Evaluations API.
// Supports default values and evaluation semantics (execute_all, deny_on_first_deny, permit_on_first_permit).
func (c *Client) Evaluations(ctx context.Context, req EvaluationsRequest) (EvaluationsResponse, error) {
	semantic := ExecuteAll
	if req.Options != nil && req.Options.Semantic != "" {
		switch req.Options.Semantic {
		case ExecuteAll, DenyOnFirstDeny, PermitOnFirstPermit:
			semantic = req.Options.Semantic
		default:
			return EvaluationsResponse{}, fmt.Errorf("unsupported evaluation semantic: %q", req.Options.Semantic)
		}
	}

	results := make([]Decision, 0, len(req.Evaluations))

	for _, eval := range req.Evaluations {
		// Apply defaults from top-level request
		merged := mergeEvaluation(eval, req)

		decision, err := c.Evaluate(ctx, merged)
		if err != nil {
			return EvaluationsResponse{}, err
		}

		results = append(results, decision)

		// Apply semantic short-circuiting
		switch semantic {
		case ExecuteAll:
			// Continue processing all evaluations
		case DenyOnFirstDeny:
			if !decision.Decision {
				return EvaluationsResponse{Evaluations: results}, nil
			}
		case PermitOnFirstPermit:
			if decision.Decision {
				return EvaluationsResponse{Evaluations: results}, nil
			}
		}
	}

	return EvaluationsResponse{Evaluations: results}, nil
}

// mergeEvaluation applies defaults from the batch request to an individual evaluation.
func mergeEvaluation(eval EvaluationRequest, req EvaluationsRequest) EvaluationRequest {
	result := eval

	// Apply subject default if not specified in evaluation
	if eval.Subject.Type == "" && req.Subject != nil {
		result.Subject = *req.Subject
	}

	// Apply resource default if not specified in evaluation
	if eval.Resource.Type == "" && req.Resource != nil {
		result.Resource = *req.Resource
	}

	// Apply action default if not specified in evaluation
	if eval.Action.Name == "" && req.Action != nil {
		result.Action = *req.Action
	}

	// Merge context (evaluation context overrides defaults)
	if req.Context != nil || eval.Context != nil {
		merged := make(Context)
		maps.Copy(merged, req.Context)
		maps.Copy(merged, eval.Context)
		result.Context = merged
	}

	return result
}
