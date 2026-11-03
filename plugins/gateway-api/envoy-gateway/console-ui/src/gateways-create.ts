import { mountCreateForm, resyncDropdown } from './create-scaffold.ts';
import {
  buildGatewayBody,
  clusterIssuerControlHtml,
  validateForm,
} from './gateways-form.ts';

await mountCreateForm({
  intro:
    'Configure a Gateway. It uses the eg GatewayClass and accepts routes from all namespaces.',
  resource: {
    group: 'gateway.networking.k8s.io',
    version: 'v1',
    resource: 'gateways',
  },
  buildBody: buildGatewayBody,
  validate: validateForm,
  onReady: () => {
    wireHttpsToggle();
    wireTlsModeToggle();
    void loadClusterIssuers(); // the markup holds the free-text field until then
  },
});

// Replaces the current control (the text field, or the dropdown around the
// select) rather than its contents: nldd-form-field watches only its own
// children, so a control swapped inside a wrapper would be left unlabeled.
function renderClusterIssuerControl(issuers: string[]): void {
  const current = document.getElementById('cluster-issuer');
  const control = current?.closest('nldd-dropdown') ?? current;
  if (!control) return;
  const template = document.createElement('template');
  template.innerHTML = clusterIssuerControlHtml(issuers);
  const next = template.content.firstElementChild as HTMLElement | null;
  if (!next) return;
  control.replaceWith(next);
  resyncDropdown(next.matches('nldd-dropdown') ? next : null);
}

// Lists cert-manager ClusterIssuers and swaps the free-text field for a dropdown
// when any exist. On failure (cert-manager not installed, or the list is denied)
// the free-text field stays, so the form still works.
async function loadClusterIssuers(): Promise<void> {
  try {
    const result = await window.fundament.k8s.list<{
      metadata?: { name?: string };
    }>({
      group: 'cert-manager.io',
      version: 'v1',
      resource: 'clusterissuers',
    });
    const names = (result.items ?? [])
      .map((i) => i.metadata?.name)
      .filter((n): n is string => Boolean(n));
    if (names.length > 0) renderClusterIssuerControl(names);
  } catch {
    // Leave the free-text field in place.
  }
}

// Show the TLS fieldset only when HTTPS is enabled.
function wireHttpsToggle(): void {
  const checkbox = document.getElementById('https-enabled') as HTMLElement & {
    checked?: boolean;
  };
  const fieldset = document.getElementById('https-fieldset') as HTMLElement;
  const apply = () => (fieldset.hidden = !checkbox.checked);
  checkbox.addEventListener('change', apply);
  apply();
}

// Swap the secret-name vs cluster-issuer field with the certificate source.
function wireTlsModeToggle(): void {
  const select = document.getElementById('tls-mode') as HTMLSelectElement;
  const secretField = document.getElementById(
    'tls-secret-field',
  ) as HTMLElement;
  const issuerField = document.getElementById(
    'cluster-issuer-field',
  ) as HTMLElement;
  const apply = () => {
    const certManager = select.value === 'certManager';
    secretField.hidden = certManager;
    issuerField.hidden = !certManager;
  };
  select.addEventListener('change', apply);
  apply();
}
