// TLSRoute create-form logic (Gateway API v1alpha2). Routes TLS connections by
// SNI hostname to one backend Service (passthrough — no certificate here).

import {
  buildBackendRefs,
  buildParentRefs,
  listValue,
  trimmedValue,
  type BackendRef,
  type ParentRef,
} from './form-helpers.ts';

export interface TLSRouteBody {
  apiVersion: 'gateway.networking.k8s.io/v1alpha2';
  kind: 'TLSRoute';
  metadata: { name: string; namespace: string };
  spec: {
    parentRefs: ParentRef[];
    hostnames?: string[];
    rules: Array<{ backendRefs: BackendRef[] }>;
  };
}

export function buildTLSRouteBody(
  root: ParentNode,
  namespace: string,
): TLSRouteBody {
  const spec: TLSRouteBody['spec'] = {
    parentRefs: buildParentRefs(root),
    rules: [{ backendRefs: buildBackendRefs(root) }],
  };
  const hostnames = listValue(root, 'hostnames');
  if (hostnames.length > 0) spec.hostnames = hostnames;

  return {
    apiVersion: 'gateway.networking.k8s.io/v1alpha2',
    kind: 'TLSRoute',
    metadata: { name: trimmedValue(root, 'name'), namespace },
    spec,
  };
}
