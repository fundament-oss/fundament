package authz

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/openfga/go-sdk/client"
)

// Client wraps the OpenFGA SDK client with an AuthZEN-compatible interface.
// See https://openid.github.io/authzen/ for the AuthZEN specification.
//
// Checks run against the store's latest authorization model; no id is pinned.
type Client struct {
	fga   *client.OpenFgaClient
	store *Store
}

// NewClient creates an authorization client that evaluates against store.
func NewClient(store *Store) *Client {
	return &Client{fga: store.fga, store: store}
}

// CanViewCluster is a convenience wrapper around Evaluate for the frequently
// used (user, can_view, cluster) tuple. Returns (false, error) if the client
// is nil so callers can wire it through configuration-optional paths without
// nil-guarding at every call site. Used by both kube-api-proxy and
// plugin-proxy — see the ClusterViewChecker interfaces in those services.
func (c *Client) CanViewCluster(ctx context.Context, userID, clusterID uuid.UUID) (bool, error) {
	if c == nil {
		return false, errors.New("openfga client not configured")
	}
	dec, err := c.Evaluate(ctx, EvaluationRequest{
		Subject:  User(userID),
		Action:   CanView(),
		Resource: Cluster(clusterID),
	})
	if err != nil {
		return false, fmt.Errorf("openfga can_view: %w", err)
	}
	return dec.Decision, nil
}

// Evaluate performs a single access evaluation following the AuthZEN Access Evaluation API.
// Returns a Decision indicating whether the subject can perform the action on the resource.
func (c *Client) Evaluate(ctx context.Context, req EvaluationRequest) (Decision, error) { //nolint:gocritic // exported signature mirrored by kube-api-proxy interfaces
	checkReq := checkRequest(&req)

	storeID, err := c.store.ID(ctx)
	if err != nil {
		return Decision{Decision: false}, err
	}

	resp, err := c.fga.Check(ctx).
		Body(checkReq).
		Options(client.ClientCheckOptions{StoreId: &storeID}).
		Execute()
	if err != nil {
		return Decision{Decision: false}, fmt.Errorf("check: %w", err)
	}

	decision := false
	if resp.Allowed != nil {
		decision = *resp.Allowed
	}

	return Decision{Decision: decision}, nil
}

// checkRequest builds the OpenFGA check for req. Resource and action properties
// are mapped into the check context under "object" and "action", so conditions
// can read them.
func checkRequest(req *EvaluationRequest) client.ClientCheckRequest {
	checkContext := map[string]any{}
	if req.Context != nil {
		checkContext = maps.Clone(req.Context)
	}

	if req.Resource.Properties != nil {
		checkContext["object"] = withProperties(checkContext["object"], req.Resource.Properties)
	}

	if req.Action.Properties != nil {
		checkContext["action"] = withProperties(checkContext["action"], req.Action.Properties)
	}

	checkReq := client.ClientCheckRequest{
		User:     req.Subject.String(),
		Relation: string(req.Action.Name),
		Object:   req.Resource.String(),
	}

	if len(checkContext) > 0 {
		checkReq.Context = &checkContext
	}

	return checkReq
}

// withProperties returns a new map, since the context may be shared between checks.
func withProperties(existing any, props map[string]any) map[string]any {
	merged := make(map[string]any)
	if m, ok := existing.(map[string]any); ok {
		maps.Copy(merged, m)
	}
	maps.Copy(merged, props)
	return merged
}

// ListObjects returns the ids of every objectType object on which subject holds
// action (an OpenFGA ListObjects call). OpenFGA bounds the result count and
// duration server-side; callers should keep the candidate set small.
func (c *Client) ListObjects(ctx context.Context, subject Object, action Action, objectType ObjectType) ([]string, error) {
	storeID, err := c.store.ID(ctx)
	if err != nil {
		return nil, err
	}

	resp, err := c.fga.ListObjects(ctx).
		Body(client.ClientListObjectsRequest{
			User:     subject.String(),
			Relation: string(action.Name),
			Type:     string(objectType),
		}).
		Options(client.ClientListObjectsOptions{StoreId: &storeID}).
		Execute()
	if err != nil {
		return nil, fmt.Errorf("list objects: %w", err)
	}

	prefix := string(objectType) + ":"
	objects := resp.GetObjects()
	ids := make([]string, 0, len(objects))
	for _, o := range objects {
		ids = append(ids, strings.TrimPrefix(o, prefix))
	}
	return ids, nil
}

// Evaluations performs batch access evaluations following the AuthZEN Access Evaluations API.
// Every evaluation is sent to OpenFGA BatchCheck, which the SDK splits into as many
// requests as the server limit needs; decisions come back in request order.
// With deny_on_first_deny or permit_on_first_permit the result is cut after the
// first matching decision. A check OpenFGA rejects fails the whole call.
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

	if len(req.Evaluations) == 0 {
		return EvaluationsResponse{Evaluations: []Decision{}}, nil
	}

	checks := make([]client.ClientBatchCheckItem, 0, len(req.Evaluations))
	for i := range req.Evaluations {
		merged := mergeEvaluation(&req.Evaluations[i], req)
		checkReq := checkRequest(&merged)
		checks = append(checks, client.ClientBatchCheckItem{
			User:          checkReq.User,
			Relation:      checkReq.Relation,
			Object:        checkReq.Object,
			Context:       checkReq.Context,
			CorrelationId: strconv.Itoa(i),
		})
	}

	storeID, err := c.store.ID(ctx)
	if err != nil {
		return EvaluationsResponse{}, err
	}

	resp, err := c.fga.BatchCheck(ctx).
		Body(client.ClientBatchCheckRequest{Checks: checks}).
		Options(client.BatchCheckOptions{StoreId: &storeID}).
		Execute()
	if err != nil {
		return EvaluationsResponse{}, fmt.Errorf("batch check: %w", err)
	}

	results := resp.GetResult()
	decisions := make([]Decision, 0, len(checks))

	for i := range checks {
		result, ok := results[strconv.Itoa(i)]
		if !ok {
			return EvaluationsResponse{}, fmt.Errorf("batch check: no result for evaluation %d", i)
		}

		if result.Error != nil {
			return EvaluationsResponse{}, fmt.Errorf("batch check: evaluation %d: %s", i, result.Error.GetMessage())
		}

		decision := Decision{Decision: result.GetAllowed()}
		decisions = append(decisions, decision)

		switch semantic {
		case ExecuteAll:
		case DenyOnFirstDeny:
			if !decision.Decision {
				return EvaluationsResponse{Evaluations: decisions}, nil
			}
		case PermitOnFirstPermit:
			if decision.Decision {
				return EvaluationsResponse{Evaluations: decisions}, nil
			}
		default:
			panic(fmt.Sprintf("unhandled evaluation semantic %q", semantic))
		}
	}

	return EvaluationsResponse{Evaluations: decisions}, nil
}

// mergeEvaluation applies defaults from the batch request to an individual evaluation.
func mergeEvaluation(eval *EvaluationRequest, req EvaluationsRequest) EvaluationRequest {
	result := *eval

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
