// HTTPRoute create-form logic. Guided form covers one rule: a path-prefix match
// forwarding to one backend Service. Multiple rules/matches/backends and header
// or method matching are advanced cases done via YAML.

import {
  buildBackendRefs,
  buildParentRefs,
  listValue,
  trimmedValue,
  type BackendRef,
  type ParentRef,
} from './form-helpers.ts';

export interface HTTPRouteBody {
  apiVersion: 'gateway.networking.k8s.io/v1';
  kind: 'HTTPRoute';
  metadata: { name: string; namespace: string };
  spec: {
    parentRefs: ParentRef[];
    hostnames?: string[];
    rules: Array<{
      matches: Array<{ path: { type: 'PathPrefix'; value: string } }>;
      backendRefs: BackendRef[];
    }>;
  };
}

export function buildHTTPRouteBody(
  root: ParentNode,
  namespace: string,
): HTTPRouteBody {
  const path = trimmedValue(root, 'path') || '/';
  const spec: HTTPRouteBody['spec'] = {
    parentRefs: buildParentRefs(root),
    rules: [
      {
        matches: [{ path: { type: 'PathPrefix', value: path } }],
        backendRefs: buildBackendRefs(root),
      },
    ],
  };
  const hostnames = listValue(root, 'hostnames');
  if (hostnames.length > 0) spec.hostnames = hostnames;

  return {
    apiVersion: 'gateway.networking.k8s.io/v1',
    kind: 'HTTPRoute',
    metadata: { name: trimmedValue(root, 'name'), namespace },
    spec,
  };
}
