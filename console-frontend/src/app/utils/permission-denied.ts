import { Code, ConnectError } from '@connectrpc/connect';
import { Action, PermissionDeniedDetailSchema, RequiredRole } from '../../generated/v1/authz_pb';

// Mirrors common/authz/denial/denial.go; sentences.json is the shared contract.
const NO_DETAIL_SENTENCE = 'You do not have permission to do this.';

/** Completes "Only <role> can ..." for each action. */
const VERBS: Partial<Record<Action, string>> = {
  [Action.VIEW_ORGANIZATION]: 'view the organization',
  [Action.EDIT_ORGANIZATION]: 'edit the organization',
  [Action.CREATE_API_KEY]: 'create API keys',
  [Action.LIST_API_KEYS]: 'list API keys',
  [Action.CREATE_CLUSTER]: 'create clusters',
  [Action.LIST_CLUSTERS]: 'list clusters',
  [Action.INVITE_MEMBER]: 'invite members',
  [Action.EDIT_MEMBER]: 'edit members',
  [Action.DELETE_MEMBER]: 'remove members',
  [Action.LIST_MEMBERS]: 'list members',
  [Action.VIEW_API_KEY]: 'view this API key',
  [Action.REVOKE_API_KEY]: 'revoke this API key',
  [Action.DELETE_API_KEY]: 'delete this API key',
  [Action.VIEW_CLUSTER]: 'view this cluster',
  [Action.EDIT_CLUSTER]: 'edit this cluster',
  [Action.DELETE_CLUSTER]: 'delete clusters',
  [Action.CREATE_NODE_POOL]: 'create node pools',
  [Action.LIST_NODE_POOLS]: 'list node pools',
  [Action.LIST_CLUSTER_NAMESPACES]: 'list the namespaces of this cluster',
  [Action.CREATE_PROJECT]: 'create projects',
  [Action.LIST_PROJECTS]: 'list projects',
  [Action.VIEW_NODE_POOL]: 'view this node pool',
  [Action.EDIT_NODE_POOL]: 'edit this node pool',
  [Action.DELETE_NODE_POOL]: 'delete this node pool',
  [Action.VIEW_PROJECT]: 'view this project',
  [Action.EDIT_PROJECT]: 'edit this project',
  [Action.DELETE_PROJECT]: 'delete this project',
  [Action.MANAGE_PROJECT_MEMBERS]: 'manage project members',
  [Action.LIST_PROJECT_MEMBERS]: 'list project members',
  [Action.CREATE_NAMESPACE]: 'create namespaces',
  [Action.LIST_NAMESPACES]: 'list namespaces',
  [Action.VIEW_PROJECT_MEMBER]: 'view this project member',
  [Action.EDIT_PROJECT_MEMBER]: 'edit this project member',
  [Action.DELETE_PROJECT_MEMBER]: 'remove this project member',
  [Action.VIEW_NAMESPACE]: 'view this namespace',
  [Action.DELETE_NAMESPACE]: 'delete this namespace',
  [Action.EDIT_PLUGIN]: 'edit plugins',
};

const ROLE_PHRASES: Partial<Record<RequiredRole, string>> = {
  [RequiredRole.ORGANIZATION_ADMIN]: 'organization admins',
  [RequiredRole.ORGANIZATION_MEMBER]: 'organization members',
  [RequiredRole.PROJECT_ADMIN]: 'project admins',
  [RequiredRole.PROJECT_MEMBER]: 'project members',
  [RequiredRole.API_KEY_OWNER]: "the key's creator and organization admins",
};

const rolePhrase = (role: RequiredRole): string =>
  ROLE_PHRASES[role] ?? 'users with the required role';

/**
 * The sentence for a permission denial, or null for any other error. A denial
 * without a detail gets the generic sentence.
 */
export default function permissionDeniedMessage(err: unknown): string | null {
  if (!(err instanceof ConnectError) || err.code !== Code.PermissionDenied) return null;
  const [detail] = err.findDetails(PermissionDeniedDetailSchema);
  if (!detail) return NO_DETAIL_SENTENCE;
  if (detail.action === Action.VIEW_LOGS) {
    return `Logs are available to ${rolePhrase(detail.requiredRole)}`;
  }
  const verb = VERBS[detail.action] ?? 'do this';
  return `Only ${rolePhrase(detail.requiredRole)} can ${verb}`;
}
