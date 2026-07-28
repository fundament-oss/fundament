// Gateway create-form logic, kept free of module-level DOM lookups so it is
// unit-testable: every function takes a form root. gateways-create.ts wires these
// to the real form and the SDK via the shared create scaffold.

import { isChecked, trimmedValue } from './form-helpers.ts';

export interface Listener {
  name: string;
  protocol: 'HTTP' | 'HTTPS';
  port: number;
  allowedRoutes: { namespaces: { from: 'All' } };
  tls?: { mode: 'Terminate'; certificateRefs: { name: string }[] };
}

export interface GatewayBody {
  apiVersion: 'gateway.networking.k8s.io/v1';
  kind: 'Gateway';
  metadata: { name: string; namespace: string; annotations?: Record<string, string> };
  spec: { gatewayClassName: 'eg'; listeners: Listener[] };
}

function httpListener(): Listener {
  return {
    name: 'http',
    protocol: 'HTTP',
    port: 80,
    allowedRoutes: { namespaces: { from: 'All' } },
  };
}

// httpsListener resolves the TLS secret name: for the cert-manager path the
// secret does not exist yet, so it is derived as "<gateway-name>-tls" (the
// convention cert-manager provisions from the Gateway annotation).
function httpsListener(root: ParentNode, name: string): Listener {
  const mode = trimmedValue(root, 'tls-mode');
  const secret = mode === 'certManager' ? `${name}-tls` : trimmedValue(root, 'tls-secret');
  return {
    name: 'https',
    protocol: 'HTTPS',
    port: 443,
    allowedRoutes: { namespaces: { from: 'All' } },
    tls: { mode: 'Terminate', certificateRefs: [{ name: secret }] },
  };
}

export function buildGatewayBody(root: ParentNode, namespace: string): GatewayBody {
  const name = trimmedValue(root, 'name');
  const listeners: Listener[] = [httpListener()];

  const metadata: GatewayBody['metadata'] = { name, namespace };

  if (isChecked(root, 'https-enabled')) {
    listeners.push(httpsListener(root, name));
    if (trimmedValue(root, 'tls-mode') === 'certManager') {
      metadata.annotations = { 'cert-manager.io/cluster-issuer': trimmedValue(root, 'cluster-issuer') };
    }
  }

  return {
    apiVersion: 'gateway.networking.k8s.io/v1',
    kind: 'Gateway',
    metadata,
    spec: { gatewayClassName: 'eg', listeners },
  };
}

export function validateForm(root: ParentNode): boolean {
  if (!trimmedValue(root, 'name')) return false;
  if (isChecked(root, 'https-enabled')) {
    const mode = trimmedValue(root, 'tls-mode');
    if (mode === 'certManager') return Boolean(trimmedValue(root, 'cluster-issuer'));
    return Boolean(trimmedValue(root, 'tls-secret'));
  }
  return true;
}
