// The FSCInstallation create-form logic, kept free of module-level DOM lookups so
// it is unit-testable: every function is scoped to a passed-in form root rather
// than reaching for `document`. create.ts wires these to the real form.

import { escapeHtml } from './shared.ts';
import type { NlddCheckboxField, NlddTextField } from './nldd-design-system.ts';

export interface Gateway {
  name: string;
  selfAddress?: string;
  certificate?: { existingSecret: string };
}

export interface FSCInstallationBody {
  apiVersion: string;
  kind: string;
  metadata: { name: string; namespace: string };
  spec: Record<string, unknown>;
}

export type GatewayKind = 'inway' | 'outway';

// Reads a control's trimmed value by id. The control may be a native <select>, a
// native <input>, or an <nldd-text-field> — all expose `.value`.
export function trimmedValue(root: ParentNode, id: string): string {
  const el = root.querySelector(`#${id}`) as { value?: string } | null;
  return (el?.value ?? '').trim();
}

export function isExternalMode(root: ParentNode): boolean {
  return trimmedValue(root, 'mode') === 'External';
}

// Shows/hides the External-only controls and flips their `required` flag.
// directory.external and certificate are required in External, forbidden in Self.
export function applyMode(root: ParentNode): void {
  const isExternal = isExternalMode(root);

  const externalFieldset = root.querySelector('#external-fieldset') as HTMLElement | null;
  if (externalFieldset) externalFieldset.hidden = !isExternal;

  root.querySelectorAll<NlddTextField>('[data-external-required]').forEach((el) => {
    el.required = isExternal;
  });
  // Per-gateway certificate overrides are only valid in External mode.
  root.querySelectorAll<HTMLElement>('.gw-cert-field').forEach((el) => {
    el.hidden = !isExternal;
  });
}

// — Validation ————————————————————————————————————————————————————————————————
// nldd-text-field exposes no constraint validity and doesn't forward
// pattern/maxlength to its inner <input>, so form.reportValidity() can't see it.
// Validate from the element's attributes and surface errors via the linked
// nldd-form-field-error-text.

// Applies a `pattern` attribute the way a native <input> would: anchored at both
// ends. A malformed pattern is a bug in the markup, not something the user typed —
// it must not throw out of the submit handler (which would leave the button stuck
// disabled), so it is logged and treated as satisfied.
function matchesPattern(value: string, pattern: string): boolean {
  try {
    return new RegExp(`^(?:${pattern})$`).test(value);
  } catch (err) {
    console.error(`invalid pattern ${JSON.stringify(pattern)} on a form field`, err);
    return true;
  }
}

function setFieldError(el: NlddTextField, message: string): void {
  el.setAttribute('invalid', '');
  const errId = el.getAttribute('error-message');
  const errEl = errId ? el.ownerDocument.getElementById(errId) : null;
  if (errEl && message) errEl.textContent = message;
}

export function validateTextField(el: NlddTextField): boolean {
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
  if (pattern && !matchesPattern(value, pattern)) {
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

// Validates every text field in the form, focusing the first invalid one.
export function validateForm(root: ParentNode): boolean {
  const fields = [...root.querySelectorAll<NlddTextField>('nldd-text-field')];
  const invalid = fields.filter((el) => !validateTextField(el));
  invalid[0]?.focus();
  return invalid.length === 0;
}

// — Body building —————————————————————————————————————————————————————————————

// Rows without a name are treated as blank and dropped, so an accidentally added
// gateway row doesn't block submission.
export function gatherGateways(
  root: ParentNode,
  kind: GatewayKind,
  isExternal: boolean,
): Gateway[] {
  return [...root.querySelectorAll<HTMLElement>(`#${kind}s .gateway-row`)]
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

// Assigns `value` onto `obj[key]` only when truthy, keeping the payload minimal.
function setIf(obj: Record<string, unknown>, key: string, value: unknown): void {
  if (value) obj[key] = value;
}

export function buildBody(root: ParentNode, namespace: string): FSCInstallationBody {
  const mode = trimmedValue(root, 'mode');
  const isExternal = mode === 'External';

  const postgres: Record<string, unknown> = { storageClass: trimmedValue(root, 'pg-storageClass') };
  const instances = trimmedValue(root, 'pg-instances');
  if (instances) postgres.instances = Number(instances);
  setIf(postgres, 'image', trimmedValue(root, 'pg-image'));
  setIf(postgres, 'storageSize', trimmedValue(root, 'pg-storageSize'));

  const directory: Record<string, unknown> = { mode };
  const spec: Record<string, unknown> = {
    groupID: trimmedValue(root, 'groupID'),
    peerID: trimmedValue(root, 'peerID'),
    directory,
    postgres,
  };

  if (isExternal) {
    directory.external = {
      address: trimmedValue(root, 'ext-address'),
      peerID: trimmedValue(root, 'ext-peerID'),
      trustAnchor: {
        name: trimmedValue(root, 'ext-ta-name'),
        key: trimmedValue(root, 'ext-ta-key') || 'ca.crt',
      },
    };
    const cert = trimmedValue(root, 'certificate');
    if (cert) spec.certificate = { existingSecret: cert };
  }

  setIf(spec, 'managerAddress', trimmedValue(root, 'managerAddress'));
  setIf(spec, 'controllerURL', trimmedValue(root, 'controllerURL'));

  // nldd-checkbox-field is not form-associated, so the grant list is read off the
  // elements directly rather than via an input[name]:checked query. Both `checked`
  // and `value` are declared reactive properties on NLDDCheckboxField.
  const grants = [
    ...root.querySelectorAll<NlddCheckboxField>('nldd-checkbox-field[name="autoSignGrants"]'),
  ]
    .filter((el) => el.checked)
    .map((el) => el.value);
  if (grants.length) spec.autoSignGrants = grants;

  const inways = gatherGateways(root, 'inway', isExternal);
  if (inways.length) spec.inways = inways;
  const outways = gatherGateways(root, 'outway', isExternal);
  if (outways.length) spec.outways = outways;

  return {
    apiVersion: 'openfsc.fundament.io/v1',
    kind: 'FSCInstallation',
    metadata: { name: trimmedValue(root, 'name'), namespace },
    spec,
  };
}

// — Markup builders ———————————————————————————————————————————————————————————
// Kept here (rather than inline in create.ts) so the generated markup is covered
// by the same tests that exercise the readers above.

// The namespace control is fed by the host (project namespaces). When the host has
// none (e.g. organization-level view), fall back to a text field. Both keep
// id/name "namespace", so trimmedValue reads work in either shape.
export function namespaceFieldHtml(namespaces: string[] | undefined): string {
  if (Array.isArray(namespaces) && namespaces.length > 0) {
    return (
      `<nldd-form-field label="Namespace">` +
      `<nldd-dropdown><select id="namespace" name="namespace" required aria-label="Namespace">` +
      namespaces.map((n) => `<option value="${escapeHtml(n)}">${escapeHtml(n)}</option>`).join('') +
      `</select></nldd-dropdown></nldd-form-field>`
    );
  }
  return (
    `<nldd-form-field label="Namespace">` +
    `<nldd-text-field id="namespace" name="namespace" required
       pattern="[a-z0-9]([a-z0-9\\-]*[a-z0-9])?"
       data-error="Use lowercase letters, digits and dashes."
       placeholder="my-namespace" error-message="namespace-error"></nldd-text-field>` +
    `<nldd-form-field-error-text id="namespace-error">Enter a valid namespace.</nldd-form-field-error-text>` +
    `</nldd-form-field>`
  );
}

export function gatewayRowHtml(kind: GatewayKind, seq: number): string {
  const nameErrId = `gw-${kind}-${seq}-name-error`;
  const selfErrId = `gw-${kind}-${seq}-self-error`;
  const selfAddress =
    kind === 'inway'
      ? `<nldd-form-field label="Self address" style="flex: 1 1 16rem">
           <nldd-text-field class="gw-self" type="url" pattern="https://.+"
             data-error="Must start with https://"
             placeholder="https://inway.example.com" error-message="${selfErrId}"></nldd-text-field>
           <nldd-form-field-error-text id="${selfErrId}">Must start with https://</nldd-form-field-error-text>
         </nldd-form-field>`
      : '';
  return `
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
}
