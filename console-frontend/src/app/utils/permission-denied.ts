import { Code, ConnectError } from '@connectrpc/connect';
import { Action } from '../../generated/v1/actions_pb';
import { PermissionDeniedDetailSchema, RequiredRole } from '../../generated/v1/authz_pb';

// Mirrors common/authz/denial/denial.go; sentences.json is the shared contract.
const NO_DETAIL_SENTENCE = 'You do not have permission to do this.';

/** The message of a denial that says nothing of its own. */
const NEUTRAL_MESSAGE = 'permission denied';

/** What connect-es writes for a 403 without a Connect error body, as from a gateway. */
const CLIENT_MADE_FORBIDDEN = 'HTTP 403';

/** Completes "Only <role> can ..." for each action. */
const VERBS: Partial<Record<Action, string>> = {
  [Action.ORGANIZATION_CAN_VIEW]: 'view the organization',
  [Action.ORGANIZATION_CAN_EDIT]: 'edit the organization',
  [Action.ORGANIZATION_CAN_CREATE_APIKEY]: 'create API keys',
  [Action.ORGANIZATION_CAN_LIST_APIKEYS]: 'list API keys',
  [Action.ORGANIZATION_CAN_CREATE_CLUSTER]: 'create clusters',
  [Action.ORGANIZATION_CAN_LIST_CLUSTERS]: 'list clusters',
  [Action.ORGANIZATION_CAN_INVITE_MEMBER]: 'invite members',
  [Action.ORGANIZATION_CAN_EDIT_MEMBER]: 'edit members',
  [Action.ORGANIZATION_CAN_DELETE_MEMBER]: 'remove members',
  [Action.ORGANIZATION_CAN_LIST_MEMBERS]: 'list members',
  [Action.API_KEY_CAN_VIEW]: 'view this API key',
  [Action.API_KEY_CAN_EDIT]: 'revoke this API key',
  [Action.API_KEY_CAN_DELETE]: 'delete this API key',
  [Action.CLUSTER_CAN_VIEW]: 'view this cluster',
  [Action.CLUSTER_CAN_EDIT]: 'edit this cluster',
  [Action.CLUSTER_CAN_DELETE]: 'delete clusters',
  [Action.CLUSTER_CAN_CREATE_NODE_POOL]: 'create node pools',
  [Action.CLUSTER_CAN_LIST_NODE_POOLS]: 'list node pools',
  [Action.CLUSTER_CAN_LIST_NAMESPACES]: 'list the namespaces of this cluster',
  [Action.CLUSTER_CAN_CREATE_PROJECT]: 'create projects',
  [Action.CLUSTER_CAN_LIST_PROJECTS]: 'list projects',
  [Action.NODE_POOL_CAN_VIEW]: 'view this node pool',
  [Action.NODE_POOL_CAN_EDIT]: 'edit this node pool',
  [Action.NODE_POOL_CAN_DELETE]: 'delete this node pool',
  [Action.PROJECT_CAN_VIEW]: 'view this project',
  [Action.PROJECT_CAN_EDIT]: 'edit this project',
  [Action.PROJECT_CAN_DELETE]: 'delete this project',
  [Action.PROJECT_CAN_MANAGE_MEMBERS]: 'manage project members',
  [Action.PROJECT_CAN_LIST_MEMBERS]: 'list project members',
  [Action.PROJECT_CAN_CREATE_NAMESPACE]: 'create namespaces',
  [Action.PROJECT_CAN_LIST_NAMESPACES]: 'list namespaces',
  [Action.PROJECT_MEMBER_CAN_VIEW]: 'view this project member',
  [Action.PROJECT_MEMBER_CAN_EDIT]: 'edit this project member',
  [Action.PROJECT_MEMBER_CAN_DELETE]: 'remove this project member',
  [Action.NAMESPACE_CAN_VIEW]: 'view this namespace',
  [Action.NAMESPACE_CAN_DELETE]: 'delete this namespace',
  [Action.ORGANIZATION_CAN_CREATE_PLUGIN]: 'create plugins',
  [Action.PLUGIN_CAN_EDIT]: 'edit plugins',
  [Action.PLUGIN_CAN_DELETE]: 'delete plugins',
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
 * without a detail shows the server's message if it says something, else the
 * general sentence.
 */
export default function permissionDeniedMessage(err: unknown): string | null {
  if (!(err instanceof ConnectError) || err.code !== Code.PermissionDenied) return null;
  const [detail] = err.findDetails(PermissionDeniedDetailSchema);
  if (!detail) {
    const message = err.rawMessage;
    return !message || message === NEUTRAL_MESSAGE || message === CLIENT_MADE_FORBIDDEN
      ? NO_DETAIL_SENTENCE
      : message;
  }
  if (detail.action === Action.CLUSTER_CAN_VIEW_LOGS) {
    return `Logs are available to ${rolePhrase(detail.requiredRole)}`;
  }
  const verb = VERBS[detail.action] ?? 'do this';
  return `Only ${rolePhrase(detail.requiredRole)} can ${verb}`;
}
