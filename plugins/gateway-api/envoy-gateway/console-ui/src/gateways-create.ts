import { mountCreateForm } from './create-scaffold.ts';
import { buildGatewayBody, validateForm } from './gateways-form.ts';

await mountCreateForm({
  intro: 'Configure a Gateway. It uses the eg GatewayClass and accepts routes from all namespaces.',
  resource: { group: 'gateway.networking.k8s.io', version: 'v1', resource: 'gateways' },
  buildBody: buildGatewayBody,
  validate: validateForm,
  onReady: () => {
    wireHttpsToggle();
    wireTlsModeToggle();
  },
});

// Show the TLS fieldset only when HTTPS is enabled.
function wireHttpsToggle(): void {
  const checkbox = document.getElementById('https-enabled') as HTMLElement & { checked?: boolean };
  const fieldset = document.getElementById('https-fieldset') as HTMLElement;
  const apply = () => (fieldset.hidden = !checkbox.checked);
  checkbox.addEventListener('change', apply);
  apply();
}

// Swap the secret-name vs cluster-issuer field with the certificate source.
function wireTlsModeToggle(): void {
  const select = document.getElementById('tls-mode') as HTMLSelectElement;
  const secretField = document.getElementById('tls-secret-field') as HTMLElement;
  const issuerField = document.getElementById('cluster-issuer-field') as HTMLElement;
  const apply = () => {
    const certManager = select.value === 'certManager';
    secretField.hidden = certManager;
    issuerField.hidden = !certManager;
  };
  select.addEventListener('change', apply);
  apply();
}
