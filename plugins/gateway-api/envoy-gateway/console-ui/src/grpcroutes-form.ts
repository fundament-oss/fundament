// GRPCRoute create-form logic. Guided form forwards all gRPC traffic on the
// attached Gateway to one backend Service; method/header matching is an advanced
// (YAML) case.

import {
  buildBackendRefs,
  buildParentRefs,
  listValue,
  trimmedValue,
  type BackendRef,
  type ParentRef,
} from './form-helpers.ts';

export interface GRPCRouteBody {
  apiVersion: 'gateway.networking.k8s.io/v1';
  kind: 'GRPCRoute';
  metadata: { name: string; namespace: string };
  spec: {
    parentRefs: ParentRef[];
    hostnames?: string[];
    rules: Array<{ backendRefs: BackendRef[] }>;
  };
}

export function buildGRPCRouteBody(root: ParentNode, namespace: string): GRPCRouteBody {
  const spec: GRPCRouteBody['spec'] = {
    parentRefs: buildParentRefs(root),
    rules: [{ backendRefs: buildBackendRefs(root) }],
  };
  const hostnames = listValue(root, 'hostnames');
  if (hostnames.length > 0) spec.hostnames = hostnames;

  return {
    apiVersion: 'gateway.networking.k8s.io/v1',
    kind: 'GRPCRoute',
    metadata: { name: trimmedValue(root, 'name'), namespace },
    spec,
  };
}
