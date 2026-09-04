// TCPRoute create-form logic (Gateway API v1alpha2). Forwards a Gateway listener
// to one backend Service — TCP is L4, so there are no hostnames or path matches.

import {
  buildBackendRefs,
  buildParentRefs,
  trimmedValue,
  type BackendRef,
  type ParentRef,
} from './form-helpers.ts';

export interface TCPRouteBody {
  apiVersion: 'gateway.networking.k8s.io/v1alpha2';
  kind: 'TCPRoute';
  metadata: { name: string; namespace: string };
  spec: {
    parentRefs: ParentRef[];
    rules: Array<{ backendRefs: BackendRef[] }>;
  };
}

export function buildTCPRouteBody(
  root: ParentNode,
  namespace: string,
): TCPRouteBody {
  return {
    apiVersion: 'gateway.networking.k8s.io/v1alpha2',
    kind: 'TCPRoute',
    metadata: { name: trimmedValue(root, 'name'), namespace },
    spec: {
      parentRefs: buildParentRefs(root),
      rules: [{ backendRefs: buildBackendRefs(root) }],
    },
  };
}
