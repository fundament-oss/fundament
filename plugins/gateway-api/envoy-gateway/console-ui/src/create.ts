import { loadSdk, loadNlddDesignSystem, navigateToDetail, navigateBack } from './shared.ts';
import { buildGatewayBody, namespaceFieldHtml, trimmedValue, validateForm } from './form.ts';
import type { NlddButton } from './nldd-design-system.ts';
import type { InitContext } from './sdk.ts';

const intro = document.getElementById('intro') as HTMLElement;
const form = document.getElementById('form') as HTMLFormElement;
const errorBox = document.getElementById('error') as HTMLElement;
const submitButton = document.getElementById('submit') as NlddButton;

document.getElementById('back')!.addEventListener('click', () => navigateBack());

let ctx: InitContext | null;
try {
  await Promise.all([loadSdk(), loadNlddDesignSystem()]);
  ctx = await window.fundament.init;
} catch (err) {
  intro.textContent = `Failed to load the plugin SDK: ${err instanceof Error ? err.message : err}`;
  ctx = null;
}

if (ctx) {
  intro.textContent = 'Configure a Gateway. It uses the eg GatewayClass and accepts routes from all namespaces.';
  renderNamespaceControl(ctx.namespaces);
  wireHttpsToggle();
  wireTlsModeToggle();
  form.hidden = false;
}

function resyncDropdown(dropdown: HTMLElement | null): void {
  const apply = () =>
    dropdown?.shadowRoot?.querySelector('slot')?.dispatchEvent(new Event('slotchange'));
  (dropdown as (HTMLElement & { updateComplete?: Promise<unknown> }) | null)?.updateComplete?.then?.(apply);
  requestAnimationFrame(apply);
}

function renderNamespaceControl(namespaces: string[] | undefined): void {
  const field = document.getElementById('namespace-field') as HTMLElement;
  field.innerHTML = namespaceFieldHtml(namespaces);
  resyncDropdown(field.querySelector('nldd-dropdown'));
}

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

// nldd-text-field's inner <input> is in shadow DOM and can't reach the light-DOM
// form, so route Enter to the submit button to restore native Enter-to-submit.
form.addEventListener('keydown', (e) => {
  const target = e.target as HTMLElement | null;
  if (e.key === 'Enter' && target?.tagName === 'NLDD-TEXT-FIELD') {
    e.preventDefault();
    submitButton.click();
  }
});

submitButton.addEventListener('click', async () => {
  if (submitButton.disabled) return;
  errorBox.hidden = true;
  if (!validateForm(form)) {
    errorBox.textContent = 'Please fill in the required fields.';
    errorBox.hidden = false;
    return;
  }

  submitButton.disabled = true;
  try {
    const namespace = trimmedValue(form, 'namespace');
    const body = buildGatewayBody(form, namespace);
    const created = await window.fundament.k8s.create<{ metadata?: { name?: string } }>(
      { group: 'gateway.networking.k8s.io', version: 'v1', resource: 'gateways', namespace },
      body,
    );
    navigateToDetail(created?.metadata?.name ?? body.metadata.name, namespace);
  } catch (err) {
    errorBox.textContent = `Failed to create: ${err instanceof Error ? err.message : err}`;
    errorBox.hidden = false;
    submitButton.disabled = false;
  }
});
