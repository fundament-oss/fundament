import { loadSdk, loadNlds, escapeHtml, navigateToDetail, navigateBack } from './shared.ts';
import type { NlddButton, NlddCheckboxField, NlddTextField } from './nlds.ts';
import type { InitContext } from './sdk.ts';

const intro = document.getElementById('intro') as HTMLElement;
const form = document.getElementById('form') as HTMLFormElement;
const errorBox = document.getElementById('error') as HTMLElement;
const externalFieldset = document.getElementById('external-fieldset') as HTMLElement;
const submitButton = document.getElementById('submit') as NlddButton;

document.getElementById('back')!.addEventListener('click', () => navigateBack());

// The directory `mode` is a native <select>; reading it by name works whether the
// control is a plain select or one wrapped in <nldd-dropdown>.
function selectByName(name: string): HTMLSelectElement {
  return form.elements.namedItem(name) as HTMLSelectElement;
}
function trimmed(id: string): string {
  const el = document.getElementById(id) as { value?: string } | null;
  return (el?.value ?? '').trim();
}

let ctx: InitContext | null;
try {
  await Promise.all([loadSdk(), loadNlds()]);
  ctx = await window.fundament.init;
} catch (err) {
  intro.textContent = `Failed to load the plugin SDK: ${err instanceof Error ? err.message : err}`;
  ctx = null;
}

if (ctx) {
  intro.textContent = 'Declare an FSCInstallation to run an OpenFSC peer in a namespace.';
  renderNamespaceControl(ctx.namespaces);
  applyMode();
  form.hidden = false;
}

// An <nldd-dropdown> computes its visible label on `slotchange`, which can fire
// before programmatically-inserted <option>s exist. Re-dispatch it once the
// component has rendered (mirrors console-frontend's dropdown-sync.directive.ts).
function resyncDropdown(dropdown: HTMLElement | null): void {
  const apply = () =>
    dropdown?.shadowRoot?.querySelector('slot')?.dispatchEvent(new Event('slotchange'));
  (dropdown as (HTMLElement & { updateComplete?: Promise<unknown> }) | null)?.updateComplete?.then?.(apply);
  requestAnimationFrame(apply);
}

// The namespace control is fed by the host (project namespaces). When the host has
// none (e.g. organization-level view), fall back to a text field. Both keep
// id/name "namespace", so reads work in either shape.
function renderNamespaceControl(namespaces: string[] | undefined): void {
  const field = document.getElementById('namespace-field') as HTMLElement;
  if (Array.isArray(namespaces) && namespaces.length > 0) {
    field.innerHTML =
      `<nldd-form-field label="Namespace">` +
      `<nldd-dropdown><select id="namespace" name="namespace" required aria-label="Namespace">` +
      namespaces.map((n) => `<option value="${escapeHtml(n)}">${escapeHtml(n)}</option>`).join('') +
      `</select></nldd-dropdown></nldd-form-field>`;
    resyncDropdown(field.querySelector('nldd-dropdown'));
  } else {
    field.innerHTML =
      `<nldd-form-field label="Namespace">` +
      `<nldd-text-field id="namespace" name="namespace" required
         pattern="[a-z0-9]([a-z0-9\\-]*[a-z0-9])?"
         data-error="Use lowercase letters, digits and dashes."
         placeholder="my-namespace" error-message="namespace-error"></nldd-text-field>` +
      `<nldd-form-field-error-text id="namespace-error">Enter a valid namespace.</nldd-form-field-error-text>` +
      `</nldd-form-field>`;
  }
}

function applyMode(): void {
  const isExternal = selectByName('mode').value === 'External';
  externalFieldset.hidden = !isExternal;
  // directory.external and certificate are required in External, forbidden in Self.
  document.querySelectorAll<NlddTextField>('[data-external-required]').forEach((el) => {
    el.required = isExternal;
  });
  // Per-gateway certificate overrides are only valid in External mode.
  document.querySelectorAll<HTMLElement>('.gw-cert-field').forEach((el) => {
    el.hidden = !isExternal;
  });
}

let gatewaySeq = 0;

function addGateway(kind: 'inway' | 'outway'): void {
  const seq = gatewaySeq++;
  const nameErrId = `gw-${kind}-${seq}-name-error`;
  const selfErrId = `gw-${kind}-${seq}-self-error`;
  const row = document.createElement('div');
  row.className = 'plugin-row gateway-row';
  const selfAddress =
    kind === 'inway'
      ? `<nldd-form-field label="Self address" style="flex: 1 1 16rem">
           <nldd-text-field class="gw-self" type="url" pattern="https://.+"
             data-error="Must start with https://"
             placeholder="https://inway.example.com" error-message="${selfErrId}"></nldd-text-field>
           <nldd-form-field-error-text id="${selfErrId}">Must start with https://</nldd-form-field-error-text>
         </nldd-form-field>`
      : '';
  row.innerHTML = `
    <nldd-form-field label="Name" style="flex: 1 1 10rem">
      <nldd-text-field class="gw-name" required maxlength="30"
        pattern="[a-z0-9]([a-z0-9\\-]*[a-z0-9])?"
        data-error="Use lowercase letters, digits and dashes."
        placeholder="default" error-message="${nameErrId}"></nldd-text-field>
      <nldd-form-field-error-text id="${nameErrId}">Enter a valid name (max 30 chars).</nldd-form-field-error-text>
    </nldd-form-field>
    ${selfAddress}
    <nldd-form-field label="Certificate secret" class="gw-cert-field" style="flex: 1 1 14rem">
      <nldd-text-field class="gw-cert" placeholder="existing tls Secret name"></nldd-text-field>
    </nldd-form-field>
    <nldd-button class="gw-remove" type="button" variant="secondary" text="Remove"></nldd-button>`;
  (row.querySelector('.gw-remove') as HTMLElement).addEventListener('click', () => row.remove());
  document.getElementById(`${kind}s`)!.appendChild(row);
  applyMode();
}

interface Gateway {
  name: string;
  selfAddress?: string;
  certificate?: { existingSecret: string };
}

function gatherGateways(kind: 'inway' | 'outway', isExternal: boolean): Gateway[] {
  return [...document.querySelectorAll<HTMLElement>(`#${kind}s .gateway-row`)]
    .map((row): Gateway | null => {
      const name = (row.querySelector('.gw-name') as NlddTextField).value.trim();
      if (!name) return null;
      const gw: Gateway = { name };
      const self = (row.querySelector('.gw-self') as NlddTextField | null)?.value.trim();
      if (self) gw.selfAddress = self;
      const cert = (row.querySelector('.gw-cert') as NlddTextField | null)?.value.trim();
      if (isExternal && cert) {
        gw.certificate = { existingSecret: cert };
      }
      return gw;
    })
    .filter((g): g is Gateway => g !== null);
}

// — Validation ————————————————————————————————————————————————————————————————
// nldd-text-field exposes no constraint validity and doesn't forward
// pattern/maxlength to its inner <input>, so form.reportValidity() can't see it.
// Validate from the element's attributes and surface errors via the linked
// nldd-form-field-error-text.
function setFieldError(el: NlddTextField, message: string): void {
  el.setAttribute('invalid', '');
  const errId = el.getAttribute('error-message');
  const errEl = errId ? document.getElementById(errId) : null;
  if (errEl && message) errEl.textContent = message;
}

function validateTextField(el: NlddTextField): boolean {
  el.removeAttribute('invalid');
  // Fields inside a hidden fieldset/form-field (Self mode, Self-mode cert) are not
  // part of the submission and skip validation.
  if (el.closest('[hidden]')) return true;
  const value = (el.value ?? '').trim();
  // `required` is toggled as a property by applyMode, so read the property (the
  // attribute lags it by an async Lit reflection).
  if (el.required && !value) {
    setFieldError(el, 'This field is required.');
    return false;
  }
  if (!value) return true;
  const max = el.getAttribute('maxlength');
  if (max && value.length > Number(max)) {
    setFieldError(el, `Use at most ${max} characters.`);
    return false;
  }
  const pattern = el.getAttribute('pattern');
  if (pattern && !new RegExp(`^(?:${pattern})$`).test(value)) {
    setFieldError(el, el.getAttribute('data-error') || 'Enter a valid value.');
    return false;
  }
  if (el.getAttribute('type') === 'number') {
    const num = Number(value);
    if (!Number.isInteger(num)) {
      setFieldError(el, 'Enter a whole number.');
      return false;
    }
    const min = el.getAttribute('min');
    if (min && num < Number(min)) {
      setFieldError(el, `Enter a value of at least ${min}.`);
      return false;
    }
  }
  return true;
}

function validate(): boolean {
  const fields = [...document.querySelectorAll<NlddTextField>('nldd-text-field')];
  const invalid = fields.filter((el) => !validateTextField(el));
  invalid[0]?.focus();
  return invalid.length === 0;
}

// Assigns `value` onto `obj[key]` only when truthy, keeping the payload minimal.
function setIf(obj: Record<string, unknown>, key: string, value: unknown): void {
  if (value) obj[key] = value;
}

function buildBody(namespace: string) {
  const mode = selectByName('mode').value;
  const isExternal = mode === 'External';

  const postgres: Record<string, unknown> = { storageClass: trimmed('pg-storageClass') };
  const instances = trimmed('pg-instances');
  if (instances) postgres.instances = Number(instances);
  setIf(postgres, 'image', trimmed('pg-image'));
  setIf(postgres, 'storageSize', trimmed('pg-storageSize'));

  const directory: Record<string, unknown> = { mode };
  const spec: Record<string, unknown> = {
    groupID: trimmed('groupID'),
    peerID: trimmed('peerID'),
    directory,
    postgres,
  };

  if (isExternal) {
    directory.external = {
      address: trimmed('ext-address'),
      peerID: trimmed('ext-peerID'),
      trustAnchor: { name: trimmed('ext-ta-name'), key: trimmed('ext-ta-key') || 'ca.crt' },
    };
    const cert = trimmed('certificate');
    if (cert) spec.certificate = { existingSecret: cert };
  }

  setIf(spec, 'managerAddress', trimmed('managerAddress'));
  setIf(spec, 'controllerURL', trimmed('controllerURL'));

  const grants = [...document.querySelectorAll<NlddCheckboxField>('nldd-checkbox-field[name="autoSignGrants"]')]
    .filter((el) => el.checked)
    .map((el) => el.value);
  if (grants.length) spec.autoSignGrants = grants;

  const inways = gatherGateways('inway', isExternal);
  if (inways.length) spec.inways = inways;
  const outways = gatherGateways('outway', isExternal);
  if (outways.length) spec.outways = outways;

  return {
    apiVersion: 'openfsc.fundament.io/v1',
    kind: 'FSCInstallation',
    metadata: { name: trimmed('name'), namespace },
    spec,
  };
}

selectByName('mode').addEventListener('change', applyMode);
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
  if (!validate()) return;

  submitButton.disabled = true;
  try {
    const namespace = trimmed('namespace');
    const body = buildBody(namespace);
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
