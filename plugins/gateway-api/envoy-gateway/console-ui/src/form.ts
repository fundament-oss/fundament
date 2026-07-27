// Gateway create-form logic, kept free of module-level DOM lookups so it is
// unit-testable: every function takes a form root rather than reaching for
// `document`. create.ts wires these to the real form and the SDK.

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

// Reads a control's trimmed value by id. The control may be a native <select>,
// <input>, or an <nldd-text-field> — all expose `.value`.
export function trimmedValue(root: ParentNode, id: string): string {
  const el = root.querySelector(`#${id}`) as { value?: string } | null;
  return (el?.value ?? '').trim();
}

export function isChecked(root: ParentNode, id: string): boolean {
  const el = root.querySelector(`#${id}`) as { checked?: boolean } | null;
  return Boolean(el?.checked);
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

// Renders the namespace control: a dropdown of project namespaces when the host
// supplies them, else a free-text field (org-level route).
export function namespaceFieldHtml(namespaces?: string[]): string {
  if (namespaces && namespaces.length > 0) {
    const options = namespaces.map((n) => `<option value="${n}">${n}</option>`).join('');
    return `<nldd-form-field label="Namespace"><nldd-dropdown><select id="namespace" name="namespace" aria-label="Namespace">${options}</select></nldd-dropdown></nldd-form-field>`;
  }
  return `<nldd-form-field label="Namespace"><nldd-text-field id="namespace" name="namespace" required placeholder="default"></nldd-text-field></nldd-form-field>`;
}
