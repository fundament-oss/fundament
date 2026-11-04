// Package denial turns a structured permission denial into the sentence shown
// to users.
package denial

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"

	authzv1 "github.com/fundament-oss/fundament/authz-api/pkg/proto/gen/v1"
)

const noDetailSentence = "You do not have permission to do this."

// verbs completes "Only <role> can ..." for each action.
var verbs = map[authzv1.Action]string{
	authzv1.Action_ACTION_VIEW_ORGANIZATION:       "view the organization",
	authzv1.Action_ACTION_EDIT_ORGANIZATION:       "edit the organization",
	authzv1.Action_ACTION_CREATE_API_KEY:          "create API keys",
	authzv1.Action_ACTION_LIST_API_KEYS:           "list API keys",
	authzv1.Action_ACTION_CREATE_CLUSTER:          "create clusters",
	authzv1.Action_ACTION_LIST_CLUSTERS:           "list clusters",
	authzv1.Action_ACTION_INVITE_MEMBER:           "invite members",
	authzv1.Action_ACTION_EDIT_MEMBER:             "edit members",
	authzv1.Action_ACTION_DELETE_MEMBER:           "remove members",
	authzv1.Action_ACTION_LIST_MEMBERS:            "list members",
	authzv1.Action_ACTION_VIEW_API_KEY:            "view this API key",
	authzv1.Action_ACTION_REVOKE_API_KEY:          "revoke this API key",
	authzv1.Action_ACTION_DELETE_API_KEY:          "delete this API key",
	authzv1.Action_ACTION_VIEW_CLUSTER:            "view this cluster",
	authzv1.Action_ACTION_EDIT_CLUSTER:            "edit this cluster",
	authzv1.Action_ACTION_DELETE_CLUSTER:          "delete clusters",
	authzv1.Action_ACTION_CREATE_NODE_POOL:        "create node pools",
	authzv1.Action_ACTION_LIST_NODE_POOLS:         "list node pools",
	authzv1.Action_ACTION_LIST_CLUSTER_NAMESPACES: "list the namespaces of this cluster",
	authzv1.Action_ACTION_CREATE_PROJECT:          "create projects",
	authzv1.Action_ACTION_LIST_PROJECTS:           "list projects",
	authzv1.Action_ACTION_VIEW_NODE_POOL:          "view this node pool",
	authzv1.Action_ACTION_EDIT_NODE_POOL:          "edit this node pool",
	authzv1.Action_ACTION_DELETE_NODE_POOL:        "delete this node pool",
	authzv1.Action_ACTION_VIEW_PROJECT:            "view this project",
	authzv1.Action_ACTION_EDIT_PROJECT:            "edit this project",
	authzv1.Action_ACTION_DELETE_PROJECT:          "delete this project",
	authzv1.Action_ACTION_MANAGE_PROJECT_MEMBERS:  "manage project members",
	authzv1.Action_ACTION_LIST_PROJECT_MEMBERS:    "list project members",
	authzv1.Action_ACTION_CREATE_NAMESPACE:        "create namespaces",
	authzv1.Action_ACTION_LIST_NAMESPACES:         "list namespaces",
	authzv1.Action_ACTION_VIEW_PROJECT_MEMBER:     "view this project member",
	authzv1.Action_ACTION_EDIT_PROJECT_MEMBER:     "edit this project member",
	authzv1.Action_ACTION_DELETE_PROJECT_MEMBER:   "remove this project member",
	authzv1.Action_ACTION_VIEW_NAMESPACE:          "view this namespace",
	authzv1.Action_ACTION_DELETE_NAMESPACE:        "delete this namespace",
	authzv1.Action_ACTION_EDIT_PLUGIN:             "edit plugins",
}

// Sentence describes a denial in one sentence.
func Sentence(d *authzv1.PermissionDeniedDetail) string {
	if d == nil {
		return noDetailSentence
	}
	if d.GetAction() == authzv1.Action_ACTION_VIEW_LOGS {
		return "Logs are available to " + rolePhrase(d.GetRequiredRole())
	}
	verb, ok := verbs[d.GetAction()]
	if !ok {
		verb = "do this"
	}
	return fmt.Sprintf("Only %s can %s", rolePhrase(d.GetRequiredRole()), verb)
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
// sentence for its PermissionDeniedDetail. Other errors pass through.
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
			return resp, &deniedError{cause: cerr, sentence: Sentence(detail)}
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
