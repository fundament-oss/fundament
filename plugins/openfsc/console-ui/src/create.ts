import { loadSdk, loadNlddDesignSystem, navigateToDetail, navigateBack } from './shared.ts';
import {
  applyMode,
  buildBody,
  gatewayRowHtml,
  namespaceFieldHtml,
  trimmedValue,
  validateForm,
  type GatewayKind,
} from './form.ts';
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
  intro.textContent = 'Declare an FSCInstallation to run an OpenFSC peer in a namespace.';
  renderNamespaceControl(ctx.namespaces);
  applyMode(form);
  form.hidden = false;
}

// An <nldd-dropdown> computes its visible label on `slotchange`, which can fire
// before programmatically-inserted <option>s exist. Re-dispatch it once the
// component has rendered (mirrors console-frontend's dropdown-sync.directive.ts).
function resyncDropdown(dropdown: HTMLElement | null): void {
  const apply = () =>
    dropdown?.shadowRoot?.querySelector('slot')?.dispatchEvent(new Event('slotchange'));
  (dropdown as (HTMLElement & { updateComplete?: Promise<unknown> }) | null)?.updateComplete?.then?.(
    apply,
  );
  requestAnimationFrame(apply);
}

function renderNamespaceControl(namespaces: string[] | undefined): void {
  const field = document.getElementById('namespace-field') as HTMLElement;
  field.innerHTML = namespaceFieldHtml(namespaces);
  resyncDropdown(field.querySelector('nldd-dropdown'));
}

let gatewaySeq = 0;

function addGateway(kind: GatewayKind): void {
  const row = document.createElement('div');
  row.className = 'plugin-row gateway-row';
  row.innerHTML = gatewayRowHtml(kind, gatewaySeq++);
  (row.querySelector('.gw-remove') as HTMLElement).addEventListener('click', () => row.remove());
  document.getElementById(`${kind}s`)!.appendChild(row);
  applyMode(form);
}

(form.querySelector('#mode') as HTMLSelectElement).addEventListener('change', () => applyMode(form));
document.getElementById('add-inway')!.addEventListener('click', () => addGateway('inway'));
document.getElementById('add-outway')!.addEventListener('click', () => addGateway('outway'));

// nldd-text-field's inner <input> is in shadow DOM and can't reach the light-DOM
// form, so route Enter to the submit button to restore native Enter-to-submit.
form.addEventListener('keydown', (e) => {
  const target = e.target as HTMLElement | null;
  if (e.key === 'Enter' && target?.tagName === 'NLDD-TEXT-FIELD') {
    e.preventDefault();
    submitButton.click();
  }
});

// The submit nldd-button is type="button": its shadow-DOM <button> can't drive the
// light-DOM form, and validation here is manual anyway.
submitButton.addEventListener('click', async () => {
  // A programmatic .click() (Enter-to-submit) isn't blocked by the host's
  // `disabled` property, so guard here against a second concurrent create.
  if (submitButton.disabled) return;
  errorBox.hidden = true;
  if (!validateForm(form)) return;

  submitButton.disabled = true;
  try {
    const namespace = trimmedValue(form, 'namespace');
    const body = buildBody(form, namespace);
    const created = await window.fundament.k8s.create<{ metadata?: { name?: string } }>(
      { group: 'openfsc.fundament.io', version: 'v1', resource: 'fscinstallations', namespace },
      body,
    );
    navigateToDetail(created?.metadata?.name ?? body.metadata.name, namespace);
  } catch (err) {
    errorBox.textContent = `Failed to create: ${err instanceof Error ? err.message : err}`;
    errorBox.hidden = false;
    submitButton.disabled = false;
  }
});
