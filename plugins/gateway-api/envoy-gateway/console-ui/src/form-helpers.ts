// Generic, DOM-lookup-free helpers shared by every create form. Each function
// takes a form `root` so the form logic stays unit-testable without `document`.

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

// Splits a comma/whitespace-separated field into trimmed, non-empty entries.
export function listValue(root: ParentNode, id: string): string[] {
  return trimmedValue(root, id)
    .split(/[\s,]+/)
    .map((s) => s.trim())
    .filter(Boolean);
}

// Renders the namespace control: a dropdown of project namespaces when the host
// supplies them, else a free-text field (org-level route).
export function namespaceFieldHtml(namespaces?: string[]): string {
  if (namespaces && namespaces.length > 0) {
    const options = namespaces
      .map((n) => `<option value="${n}">${n}</option>`)
      .join('');
    return `<nldd-form-field label="Namespace"><nldd-dropdown><select id="namespace" name="namespace" aria-label="Namespace">${options}</select></nldd-dropdown></nldd-form-field>`;
  }
  return `<nldd-form-field label="Namespace"><nldd-text-field id="namespace" name="namespace" required placeholder="default" unmet="namespace-error"></nldd-text-field><nldd-validation-list><nldd-validation-item id="namespace-error">This field is required.</nldd-validation-item></nldd-validation-list></nldd-form-field>`;
}

// --- Field validation ---
//
// The form is judged on submit, from each nldd-text-field's own attributes:
// required, maxlength, pattern and, for type="number", min and max. A failure
// marks the field invalid and puts the message in the validation item its
// `unmet` names. A field inside a hidden section is not submitted, so it is
// not judged either; that is how a field is required only while it shows.

type TextField = HTMLElement & { value?: string };

// Applies `pattern` the way a native <input> does: anchored at both ends. A
// malformed pattern is a bug in the markup, so it counts as satisfied rather
// than blocking every submit.
function matchesPattern(value: string, pattern: string): boolean {
  try {
    return new RegExp(`^(?:${pattern})$`).test(value);
  } catch {
    return true;
  }
}

// The message for a field, or null when it is valid.
export function fieldError(el: TextField): string | null {
  const value = (el.value ?? '').trim();
  if (!value) {
    return el.hasAttribute('required') ? 'This field is required.' : null;
  }

  const max = el.getAttribute('maxlength');
  if (max && value.length > Number(max)) {
    return `Use at most ${max} characters.`;
  }
  const pattern = el.getAttribute('pattern');
  if (pattern && !matchesPattern(value, pattern)) {
    // Falls back to the text the validation item was written with.
    return el.getAttribute('data-error') ?? '';
  }
  if (el.getAttribute('type') === 'number') {
    const num = Number(value);
    const minAttr = el.getAttribute('min');
    const maxAttr = el.getAttribute('max');
    if (!Number.isInteger(num)) return 'Enter a whole number.';
    if (num < Number(minAttr ?? -Infinity) || num > Number(maxAttr ?? Infinity)) {
      if (minAttr === null) return `Enter a number of at most ${maxAttr}.`;
      if (maxAttr === null) return `Enter a number of at least ${minAttr}.`;
      return `Enter a number from ${minAttr} to ${maxAttr}.`;
    }
  }
  return null;
}

function showFieldError(el: TextField, message: string): void {
  el.setAttribute('invalid', '');
  const id = el.getAttribute('unmet');
  const item = id ? el.ownerDocument.getElementById(id) : null;
  if (!item) return;
  // The markup's own text is the message for a failed pattern; keep it so a
  // later "required" does not overwrite it for good.
  item.dataset.message ??= item.textContent ?? '';
  item.textContent = message || item.dataset.message;
}

// Judges every visible text field, marks the failing ones and focuses the
// first. Returns whether all of them passed.
export function validateFields(root: ParentNode): boolean {
  const fields = [...root.querySelectorAll<TextField>('nldd-text-field')];
  let firstInvalid: TextField | null = null;
  for (const el of fields) {
    el.removeAttribute('invalid');
    if (el.closest('[hidden]')) continue;
    const message = fieldError(el);
    if (message === null) continue;
    showFieldError(el, message);
    firstInvalid ??= el;
  }
  firstInvalid?.focus();
  return firstInvalid === null;
}

// --- Gateway API building blocks shared by the route/policy forms ---

export interface ParentRef {
  name: string;
  sectionName?: string;
}

// Builds parentRefs from `#parent-name` (the Gateway) and optional
// `#parent-section` (a specific listener). Routes always attach to exactly one
// Gateway in the guided form; multiple parents are an advanced (YAML) case.
export function buildParentRefs(root: ParentNode): ParentRef[] {
  const ref: ParentRef = { name: trimmedValue(root, 'parent-name') };
  const section = trimmedValue(root, 'parent-section');
  if (section) ref.sectionName = section;
  return [ref];
}

export interface BackendRef {
  name: string;
  port: number;
}

// Builds a single backendRef from `#backend-name` and `#backend-port`.
export function buildBackendRefs(root: ParentNode): BackendRef[] {
  return [
    {
      name: trimmedValue(root, 'backend-name'),
      port: Number(trimmedValue(root, 'backend-port')),
    },
  ];
}

export interface TargetRef {
  group: 'gateway.networking.k8s.io';
  kind: string;
  name: string;
}

// Builds a policy targetRef from `#target-kind` and `#target-name`. All the
// targetable kinds (Gateway, HTTPRoute, …) live in the gateway.networking.k8s.io
// group.
export function buildTargetRef(root: ParentNode): TargetRef {
  return {
    group: 'gateway.networking.k8s.io',
    kind: trimmedValue(root, 'target-kind'),
    name: trimmedValue(root, 'target-name'),
  };
}

// Common validity checks reused by route forms: a name, a parent Gateway, and a
// backend Service+port.
export function validateRoute(root: ParentNode): boolean {
  if (!trimmedValue(root, 'name')) return false;
  if (!trimmedValue(root, 'parent-name')) return false;
  if (!trimmedValue(root, 'backend-name')) return false;
  const port = Number(trimmedValue(root, 'backend-port'));
  return Number.isInteger(port) && port >= 1 && port <= 65535;
}

// Common validity checks reused by policy forms: a name and a target name.
export function validatePolicy(root: ParentNode): boolean {
  return (
    Boolean(trimmedValue(root, 'name')) &&
    Boolean(trimmedValue(root, 'target-name'))
  );
}
