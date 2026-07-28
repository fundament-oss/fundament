// ClientTrafficPolicy create-form logic (gateway.envoyproxy.io/v1alpha1). It only
// targets a Gateway. The guided form covers the downstream TLS minimum version;
// HTTP/1-2-3, connection and timeout tuning are configured via YAML.

import { buildTargetRef, trimmedValue, type TargetRef } from './form-helpers.ts';

export interface ClientTrafficPolicyBody {
  apiVersion: 'gateway.envoyproxy.io/v1alpha1';
  kind: 'ClientTrafficPolicy';
  metadata: { name: string; namespace: string };
  spec: {
    targetRefs: TargetRef[];
    tls?: { minVersion: string };
  };
}

export function buildClientTrafficPolicyBody(root: ParentNode, namespace: string): ClientTrafficPolicyBody {
  const spec: ClientTrafficPolicyBody['spec'] = { targetRefs: [buildTargetRef(root)] };

  const minVersion = trimmedValue(root, 'tls-min-version');
  if (minVersion) spec.tls = { minVersion };

  return {
    apiVersion: 'gateway.envoyproxy.io/v1alpha1',
    kind: 'ClientTrafficPolicy',
    metadata: { name: trimmedValue(root, 'name'), namespace },
    spec,
  };
}
