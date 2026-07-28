// BackendTrafficPolicy create-form logic (gateway.envoyproxy.io/v1alpha1). The
// guided form covers a local rate limit; load balancing, circuit breaking,
// health checks, retries and timeouts are configured via YAML.

import { buildTargetRef, trimmedValue, type TargetRef } from './form-helpers.ts';

export interface BackendTrafficPolicyBody {
  apiVersion: 'gateway.envoyproxy.io/v1alpha1';
  kind: 'BackendTrafficPolicy';
  metadata: { name: string; namespace: string };
  spec: {
    targetRefs: TargetRef[];
    rateLimit?: {
      type: 'Local';
      local: { rules: Array<{ limit: { requests: number; unit: string } }> };
    };
  };
}

export function buildBackendTrafficPolicyBody(root: ParentNode, namespace: string): BackendTrafficPolicyBody {
  const spec: BackendTrafficPolicyBody['spec'] = { targetRefs: [buildTargetRef(root)] };

  const requests = Number(trimmedValue(root, 'rate-requests'));
  if (Number.isInteger(requests) && requests > 0) {
    spec.rateLimit = {
      type: 'Local',
      local: { rules: [{ limit: { requests, unit: trimmedValue(root, 'rate-unit') || 'Second' } }] },
    };
  }

  return {
    apiVersion: 'gateway.envoyproxy.io/v1alpha1',
    kind: 'BackendTrafficPolicy',
    metadata: { name: trimmedValue(root, 'name'), namespace },
    spec,
  };
}
