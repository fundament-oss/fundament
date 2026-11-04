// Package denial turns a structured permission denial into the sentence shown
// to users.
package denial

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"connectrpc.com/connect"

	authzv1 "github.com/fundament-oss/fundament/authz-api/pkg/proto/gen/v1"
)

const (
	noDetailSentence = "You do not have permission to do this."
	// neutralMessage is the message of a denial that says nothing of its own.
	neutralMessage = "permission denied"
)

// clientMadeForbidden matches what connect-go writes for a 403 without a Connect
// error body, as from a gateway: "403 Forbidden" (unary) or "HTTP status 403
// Forbidden" (streaming). The reason phrase is the proxy's own.
var clientMadeForbidden = regexp.MustCompile(`^(HTTP status )?403( |$)`)

// verbs completes "Only <role> can ..." for each action.
var verbs = map[authzv1.Action]string{
	authzv1.Action_ACTION_ORGANIZATION_CAN_VIEW:           "view the organization",
	authzv1.Action_ACTION_ORGANIZATION_CAN_EDIT:           "edit the organization",
	authzv1.Action_ACTION_ORGANIZATION_CAN_CREATE_APIKEY:  "create API keys",
	authzv1.Action_ACTION_ORGANIZATION_CAN_LIST_APIKEYS:   "list API keys",
	authzv1.Action_ACTION_ORGANIZATION_CAN_CREATE_CLUSTER: "create clusters",
	authzv1.Action_ACTION_ORGANIZATION_CAN_LIST_CLUSTERS:  "list clusters",
	authzv1.Action_ACTION_ORGANIZATION_CAN_INVITE_MEMBER:  "invite members",
	authzv1.Action_ACTION_ORGANIZATION_CAN_EDIT_MEMBER:    "edit members",
	authzv1.Action_ACTION_ORGANIZATION_CAN_DELETE_MEMBER:  "remove members",
	authzv1.Action_ACTION_ORGANIZATION_CAN_LIST_MEMBERS:   "list members",
	authzv1.Action_ACTION_API_KEY_CAN_VIEW:                "view this API key",
	authzv1.Action_ACTION_API_KEY_CAN_EDIT:                "revoke this API key",
	authzv1.Action_ACTION_API_KEY_CAN_DELETE:              "delete this API key",
	authzv1.Action_ACTION_CLUSTER_CAN_VIEW:                "view this cluster",
	authzv1.Action_ACTION_CLUSTER_CAN_EDIT:                "edit this cluster",
	authzv1.Action_ACTION_CLUSTER_CAN_DELETE:              "delete clusters",
	authzv1.Action_ACTION_CLUSTER_CAN_CREATE_NODE_POOL:    "create node pools",
	authzv1.Action_ACTION_CLUSTER_CAN_LIST_NODE_POOLS:     "list node pools",
	authzv1.Action_ACTION_CLUSTER_CAN_LIST_NAMESPACES:     "list the namespaces of this cluster",
	authzv1.Action_ACTION_CLUSTER_CAN_CREATE_PROJECT:      "create projects",
	authzv1.Action_ACTION_CLUSTER_CAN_LIST_PROJECTS:       "list projects",
	authzv1.Action_ACTION_NODE_POOL_CAN_VIEW:              "view this node pool",
	authzv1.Action_ACTION_NODE_POOL_CAN_EDIT:              "edit this node pool",
	authzv1.Action_ACTION_NODE_POOL_CAN_DELETE:            "delete this node pool",
	authzv1.Action_ACTION_PROJECT_CAN_VIEW:                "view this project",
	authzv1.Action_ACTION_PROJECT_CAN_EDIT:                "edit this project",
	authzv1.Action_ACTION_PROJECT_CAN_DELETE:              "delete this project",
	authzv1.Action_ACTION_PROJECT_CAN_MANAGE_MEMBERS:      "manage project members",
	authzv1.Action_ACTION_PROJECT_CAN_LIST_MEMBERS:        "list project members",
	authzv1.Action_ACTION_PROJECT_CAN_CREATE_NAMESPACE:    "create namespaces",
	authzv1.Action_ACTION_PROJECT_CAN_LIST_NAMESPACES:     "list namespaces",
	authzv1.Action_ACTION_PROJECT_MEMBER_CAN_VIEW:         "view this project member",
	authzv1.Action_ACTION_PROJECT_MEMBER_CAN_EDIT:         "edit this project member",
	authzv1.Action_ACTION_PROJECT_MEMBER_CAN_DELETE:       "remove this project member",
	authzv1.Action_ACTION_NAMESPACE_CAN_VIEW:              "view this namespace",
	authzv1.Action_ACTION_NAMESPACE_CAN_DELETE:            "delete this namespace",
	authzv1.Action_ACTION_ORGANIZATION_CAN_CREATE_PLUGIN:  "create plugins",
	authzv1.Action_ACTION_PLUGIN_CAN_DELETE:               "delete plugins",
	authzv1.Action_ACTION_PLUGIN_CAN_EDIT:                 "edit plugins",
}

// Sentence describes a denial in one sentence.
func Sentence(d *authzv1.PermissionDeniedDetail) string {
	if d == nil {
		return noDetailSentence
	}
	if d.GetAction() == authzv1.Action_ACTION_CLUSTER_CAN_VIEW_LOGS {
		return "Logs are available to " + rolePhrase(d.GetRequiredRole())
	}
	verb, ok := verbs[d.GetAction()]
	if !ok {
		verb = "do this"
	}
	return fmt.Sprintf("Only %s can %s", rolePhrase(d.GetRequiredRole()), verb)
}

// sentenceWithoutDetail describes a denial that has no detail: the server's own
// message if it says something, else the general sentence. The message is shown
// to users, so a denial's message must be fit for them.
func sentenceWithoutDetail(message string) string {
	if message == "" || message == neutralMessage || clientMadeForbidden.MatchString(message) {
		return noDetailSentence
	}
	return message
}

const fallbackRolePhrase = "users with the required role"

func rolePhrase(r authzv1.RequiredRole) string {
	switch r {
	case authzv1.RequiredRole_REQUIRED_ROLE_ORGANIZATION_ADMIN:
		return "organization admins"
	case authzv1.RequiredRole_REQUIRED_ROLE_ORGANIZATION_MEMBER:
		return "organization members"
	case authzv1.RequiredRole_REQUIRED_ROLE_PROJECT_ADMIN:
		return "project admins"
	case authzv1.RequiredRole_REQUIRED_ROLE_PROJECT_MEMBER:
		return "project members"
	case authzv1.RequiredRole_REQUIRED_ROLE_API_KEY_OWNER:
		return "the key's creator and organization admins"
	case authzv1.RequiredRole_REQUIRED_ROLE_UNSPECIFIED:
		return fallbackRolePhrase
	default:
		// A newer API can send a role this build does not know.
		return fallbackRolePhrase
	}
}

// Interceptor replaces the message of a permission_denied error with the
// sentence for its PermissionDeniedDetail, or without one, with the server's
// message minus connect's code prefix. Other errors pass through.
func Interceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			resp, err := next(ctx, req)
			var cerr *connect.Error
			if !errors.As(err, &cerr) || cerr.Code() != connect.CodePermissionDenied {
				return resp, err
			}
			var detail *authzv1.PermissionDeniedDetail
			for _, d := range cerr.Details() {
				msg, verr := d.Value()
				if verr != nil {
					continue
				}
				if pd, ok := msg.(*authzv1.PermissionDeniedDetail); ok {
					detail = pd
					break
				}
			}
			sentence := sentenceWithoutDetail(cerr.Message())
			if detail != nil {
				sentence = Sentence(detail)
			}
			return resp, &deniedError{cause: cerr, sentence: sentence}
		}
	}
}

// deniedError reports only the sentence, without connect's code prefix, and
// still unwraps to the original connect error so the code is preserved.
type deniedError struct {
	cause    *connect.Error
	sentence string
}

func (e *deniedError) Error() string { return e.sentence }

func (e *deniedError) Unwrap() error { return e.cause }
