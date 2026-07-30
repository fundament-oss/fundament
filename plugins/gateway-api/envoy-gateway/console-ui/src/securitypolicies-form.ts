// SecurityPolicy create-form logic (gateway.envoyproxy.io/v1alpha1). The guided
// form covers the common CORS case; JWT/OIDC/extAuth/authorization and other
// features are configured via YAML.

import { buildTargetRef, listValue, trimmedValue, type TargetRef } from './form-helpers.ts';

export interface SecurityPolicyBody {
  apiVersion: 'gateway.envoyproxy.io/v1alpha1';
  kind: 'SecurityPolicy';
  metadata: { name: string; namespace: string };
  spec: {
    targetRefs: TargetRef[];
    cors?: { allowOrigins?: string[]; allowMethods?: string[] };
  };
}

export function buildSecurityPolicyBody(root: ParentNode, namespace: string): SecurityPolicyBody {
  const spec: SecurityPolicyBody['spec'] = { targetRefs: [buildTargetRef(root)] };

  const origins = listValue(root, 'cors-origins');
  const methods = listValue(root, 'cors-methods');
  if (origins.length > 0 || methods.length > 0) {
    spec.cors = {};
    if (origins.length > 0) spec.cors.allowOrigins = origins;
    if (methods.length > 0) spec.cors.allowMethods = methods;
  }

  return {
    apiVersion: 'gateway.envoyproxy.io/v1alpha1',
    kind: 'SecurityPolicy',
    metadata: { name: trimmedValue(root, 'name'), namespace },
    spec,
  };
}
