// Shared helpers for ceph-rook plugin templates. Plain ES module so templates
// can `import` from it.
//
// The plugin CSP has no 'unsafe-inline', which rules out onclick= and inline
// style alike: events go through addEventListener, styling through the
// <nldd-*> components in sheets and the .plugin-* classes in plugin-sdk.css.

// Loads the plugin SDK v1. The iframe runs on the plugin-proxy origin, which
// also serves the SDK, so these bare paths satisfy script-src 'self'. The /v1/
// segment tracks protocolVersion: a breaking change ships as /v2/.
export function loadSdk() {
  const link = document.createElement('link');
  link.rel = 'stylesheet';
  link.href = '/plugins/sdk/v1/plugin-sdk.css';
  document.head.appendChild(link);

  return new Promise((resolve, reject) => {
    const script = document.createElement('script');
    script.src = '/plugins/sdk/v1/plugin-sdk.js';
    script.onload = () => resolve(window.fundament);
    script.onerror = () => reject(new Error('failed to load plugin-sdk.js'));
    document.head.appendChild(script);
  });
}

// Loads the shared NLDD Design System bundle served next to the SDK
// (FUN-18) and mirrors the body's light/dark class into data-scheme.
// Memoized and loaded lazily from the sheet-opening handlers, so a host
// without the bundle still serves the read-only views; the stylesheet is
// awaited too, or sheets render unstyled with the error swallowed.
let nlddLoad;
let nlddThemeSync = false;
export function ensureNldd() {
  nlddLoad ??= (() => {
    if (!nlddThemeSync) {
      nlddThemeSync = true;
      const sync = () => {
        document.documentElement.setAttribute(
          'data-scheme',
          document.body.classList.contains('dark') ? 'dark' : 'light',
        );
      };
      sync();
      new MutationObserver(sync).observe(document.body, {
        attributes: true,
        attributeFilter: ['class'],
      });
    }

    const settled = (el, what) => new Promise((resolve, reject) => {
      el.addEventListener('load', () => resolve(), { once: true });
      el.addEventListener('error', () => reject(new Error(`failed to load ${what}`)), { once: true });
    });
    const link = document.createElement('link');
    link.rel = 'stylesheet';
    link.href = '/plugins/sdk/v1/nldd-design-system.css';
    const css = settled(link, 'nldd-design-system.css');
    document.head.appendChild(link);
    const script = document.createElement('script');
    script.src = '/plugins/sdk/v1/nldd-design-system.js';
    const js = settled(script, 'nldd-design-system.js');
    document.head.appendChild(script);
    return Promise.all([css, js]).then(() => undefined);
  })().catch((err) => {
    // Transient asset failures retry on the next click instead of
    // disabling every sheet until a reload.
    nlddLoad = undefined;
    throw err;
  });
  return nlddLoad;
}

// Surfaces a sheet-open failure without killing the page: one reused
// .plugin-error at the top of the page card.
export function showSheetError(err) {
  const card = document.querySelector('.plugin-card');
  let box = card?.querySelector('[data-role="sheet-error"]');
  if (!box && card) {
    box = document.createElement('div');
    box.className = 'plugin-error';
    box.dataset.role = 'sheet-error';
    card.prepend(box);
  }
  if (box) box.textContent = `Cannot open the editor: ${err?.message ?? err}`;
}

// Opens a right-hand <nldd-sheet> appended to the document root (it is a
// <dialog>; inside the content flow it would steal layout height). Loads the
// design-system bundle on first use; on failure it surfaces the error and
// returns null, so callers bail with `if (!sheet) return`. Otherwise returns
// the content container and a close(); the element removes itself after the
// closing animation.
//
// The host sizes the iframe to this document's height, so on a short page
// the sheet, confined to the iframe's viewport, would render a few rows
// tall; growing the body makes the host grow the iframe. CSSOM property
// assignment, not style attributes: the CSP blocks only the latter.
let openSheetCount = 0;
let preSheetMinHeight = '';

export async function openSheet({ label, width = '480px', minHeight = '640px' }) {
  try {
    await ensureNldd();
  } catch (err) {
    showSheetError(err);
    return null;
  }
  // Only the outermost open/last close touch the body: chained sheets
  // would otherwise capture the inflated value as "previous" and latch it.
  if (openSheetCount === 0) {
    preSheetMinHeight = document.body.style.minHeight;
    document.body.style.minHeight = minHeight;
  }
  openSheetCount += 1;

  const sheet = document.createElement('nldd-sheet');
  sheet.setAttribute('placement', 'right');
  sheet.setAttribute('width', width);
  sheet.setAttribute('accessible-label', label);
  const body = document.createElement('div');
  body.className = 'plugin-card';
  body.style.maxHeight = '100dvh';
  body.style.overflowY = 'auto';
  if (label) {
    const title = document.createElement('nldd-title');
    title.setAttribute('size', '3');
    const heading = document.createElement('h2');
    heading.textContent = label;
    title.appendChild(heading);
    const spacer = document.createElement('nldd-spacer');
    spacer.setAttribute('size', '16');
    body.append(title, spacer);
  }
  sheet.appendChild(body);
  document.body.appendChild(sheet);
  sheet.addEventListener('close', () => {
    openSheetCount -= 1;
    if (openSheetCount === 0) document.body.style.minHeight = preSheetMinHeight;
    sheet.remove();
  });
  sheet.show();
  return { body, close: () => sheet.hide() };
}

export function escapeHtml(value) {
  if (value === null || value === undefined) return '';
  return String(value)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

export function emptyRow(colspan, message = 'No items.') {
  return `<tr><td colspan="${colspan}" class="plugin-text">${escapeHtml(message)}</td></tr>`;
}

export function errorRow(colspan, err) {
  const message = err?.message ?? String(err);
  return `<tr><td colspan="${colspan}" class="plugin-text">${escapeHtml(`Failed to load: ${message}`)}</td></tr>`;
}

// The block-level sibling of errorRow, for detail views and sheets.
export function errorBox(err, prefix = 'Failed to load') {
  return `<div class="plugin-error">${escapeHtml(`${prefix}: ${err?.message ?? err}`)}</div>`;
}

// Posts a navigate message to the parent, which resolves it relative to the
// iframe's current route — so this sends only the resource identity, and works
// from a create view too. parentOrigin falls back to '*' before init.
export function navigateToDetail(name, namespace) {
  window.parent.postMessage(
    { type: 'plugin:navigate', name, namespace },
    window.fundament?.parentOrigin ?? '*',
  );
}

// Makes each data-name row a keyboard-reachable link. Anchors must be in the DOM.
export function wireRowLinks(root) {
  root.querySelectorAll('a.row-link').forEach((link) => {
    link.addEventListener('click', (e) => {
      e.preventDefault();
      const row = link.closest('tr');
      navigateToDetail(row.dataset.name, row.dataset.namespace || undefined);
    });
  });
}

// Renders a key/value definition list from [key, value] pairs. Both halves are
// escaped here, so pass raw values — pre-escaped ones render as "&amp;".
export function renderDefList(pairs) {
  const rows = pairs
    .map(([k, v]) => `<dt>${escapeHtml(k)}</dt><dd>${escapeHtml(v)}</dd>`)
    .join('');
  return `<dl class="plugin-deflist">${rows}</dl>`;
}

// Mirrors the Kubernetes object-name pattern the CRDs enforce; returns an
// error message or null. maxLength mirrors a kind's CRD name cap where one
// exists (FileStorage caps at 56; see filestorage_types.go).
export function resourceNameError(name, maxLength = 63) {
  if (!name) return 'Please enter a name.';
  if (!/^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/.test(name)) {
    return 'Use lowercase letters, digits and dashes only.';
  }
  if (name.length > maxLength) {
    return `Use at most ${maxLength} characters.`;
  }
  return null;
}

// One nldd-form-field; an empty label leaves the field without one. errorId names the empty validation item wireSubmit
// fills when the field's check fails; a field without a check passes none.
export function formFieldHtml(label, control, { errorId, hint } = {}) {
  const errors = errorId
    ? `<nldd-validation-list><nldd-validation-item id="${errorId}"></nldd-validation-item></nldd-validation-list>`
    : '';
  const help = hint ? `<nldd-form-field-help-text>${escapeHtml(hint)}</nldd-form-field-help-text>` : '';
  return `
    <nldd-form-field${label ? ` label="${escapeHtml(label)}"` : ''}>
      ${control}
      ${errors}
      ${help}
    </nldd-form-field>`;
}

// The Save/Create and Cancel pair at the bottom of every form.
export function formActionsHtml({ submitId, submitText, cancelId }) {
  return `
    <nldd-form-actions>
      <nldd-button-group orientation="horizontal">
        <nldd-button id="${submitId}" type="submit" variant="primary" text="${escapeHtml(submitText)}"></nldd-button>
        <nldd-button id="${cancelId}" type="button" variant="secondary" text="Cancel"></nldd-button>
      </nldd-button-group>
    </nldd-form-actions>`;
}

export function errorBannerHtml(id) {
  return `<nldd-banner id="${id}" variant="critical" hidden></nldd-banner>`;
}

// A standalone critical banner, for a sheet whose data failed to load.
export function loadErrorBanner(message) {
  const banner = document.createElement('nldd-banner');
  banner.setAttribute('variant', 'critical');
  banner.setAttribute('text', message);
  return banner;
}

// Replicas dropdown shared by every consumer create/edit form. "auto" stands
// for an absent spec.replicas.
export function replicasFieldHtml(replicas) {
  const selected = replicas ? String(replicas) : 'auto';
  const label = (v) => (v === 'auto' ? 'auto (recommended)' : {
    1: '1 (no replication)',
    2: '2 replicas',
    3: '3 replicas',
  }[v]);
  const options = ['auto', '1', '2', '3']
    .map((v) => `<option value="${v}"${v === selected ? ' selected' : ''}>${label(v)}</option>`)
    .join('');
  return formFieldHtml(
    'Replicas',
    `<nldd-dropdown><select id="replicas" name="replicas">${options}</select></nldd-dropdown>`,
    { hint: 'auto derives the replica count from the number of nodes contributing disks to the cluster.' },
  );
}

// Reads the replicas dropdown into spec.replicas. null, not undefined, so a
// merge-patch removes the field when the operator switches back to auto.
export function replicasValue(form) {
  const value = form.querySelector('[name="replicas"]').value;
  return value === 'auto' ? null : Number(value);
}

// Default-StorageClass checkbox shared by the BlockStorage create and edit
// forms. At most one BlockStorage may set it; conflicts show as Degraded.
export function defaultFieldHtml(checked = false) {
  return formFieldHtml(
    '',
    `<nldd-checkbox-field name="default" label="Default StorageClass"${checked ? ' checked' : ''}></nldd-checkbox-field>`,
    { hint: 'PersistentVolumeClaims without an explicit storageClassName use this class. Only one BlockStorage may be the default; a second one degrades both until resolved.' },
  );
}

// One integer field, driven by a field descriptor (see the intFields
// descriptors in consumer-pages.js), so each field's CRD bounds live in one
// place instead of a copy per form and per validator.
export function integerFieldHtml(field, value) {
  return formFieldHtml(
    field.label,
    `<nldd-number-field id="${field.id}" name="${field.name}" min="${field.min}" max="${field.max}"
                        value="${escapeHtml(String(value))}" width="160px"></nldd-number-field>`,
    { errorId: `${field.id}-error`, hint: field.hint },
  );
}

// Bounds mirror the field's CRD validation; returns an error message or null.
export function integerFieldError(field, value) {
  if (!Number.isInteger(value) || value < field.min || value > field.max) {
    return `${field.label} must be a whole number from ${field.min} to ${field.max}.`;
  }
  return null;
}

// The element that carries `invalid` for a field: the <nldd-dropdown> around a
// <select>, or the control itself.
function fieldControl(el) {
  return el.closest('nldd-dropdown') ?? el;
}

// The one rule-less item in the field's nldd-validation-list: inside its
// nldd-form-field, or, for a group such as the disk picker, the list that
// names the control in `for`.
function validationItem(control) {
  const list =
    control.closest('nldd-form-field')?.querySelector('nldd-validation-list') ??
    document.querySelector(`nldd-validation-list[for="${control.id}"]`);
  return list?.querySelector('nldd-validation-item') ?? null;
}

function setFieldError(el, message) {
  const control = fieldControl(el);
  const item = validationItem(control);
  if (item) {
    item.textContent = message;
    control.setAttribute('unmet', item.id);
  }
  control.setAttribute('invalid', '');
}

function clearFieldError(el) {
  const control = fieldControl(el);
  control.removeAttribute('invalid');
  control.removeAttribute('unmet');
}

// Wires the shared submit flow. checks are [control, check] pairs, where check
// gets the control's trimmed value and returns an error message or null; each
// failure shows under its own field and the first failing field takes focus.
// A failing action shows in the critical errorBanner. On success the button
// keeps loading: the action closes the sheet, navigates or re-renders.
export function wireSubmit(form, { button, errorBanner, failPrefix, checks = [], action }) {
  // A field is judged on submit, and stops being wrong the moment it is edited.
  for (const [control] of checks) {
    control.addEventListener('input', () => clearFieldError(control));
    control.addEventListener('change', () => clearFieldError(control));
  }

  let busy = false;
  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    // Enter in a text field submits too, so a click is not the only way in.
    if (busy) return;
    errorBanner.hidden = true;

    let firstInvalid = null;
    for (const [control, check] of checks) {
      const message = check(String(control.value ?? '').trim());
      if (message) {
        setFieldError(control, message);
        firstInvalid ??= control;
      } else {
        clearFieldError(control);
      }
    }
    if (firstInvalid) {
      firstInvalid.focus();
      return;
    }

    busy = true;
    button.loading = true;
    try {
      await action();
    } catch (err) {
      errorBanner.setAttribute('text', `${failPrefix}: ${err?.message ?? err}`);
      errorBanner.hidden = false;
      busy = false;
      button.loading = false;
    }
  });
}

const QUANTITY_MULTIPLIERS = {
  Ki: 2 ** 10, Mi: 2 ** 20, Gi: 2 ** 30, Ti: 2 ** 40, Pi: 2 ** 50, Ei: 2 ** 60,
  n: 1e-9, u: 1e-6, m: 1e-3, '': 1, k: 1e3, M: 1e6, G: 1e9, T: 1e12, P: 1e15, E: 1e18,
};

// The Kubernetes Quantity grammar (k8s.io/apimachinery resource.Quantity):
// <signedNumber> followed by a binary SI suffix, a decimal exponent or a
// decimal SI suffix. The exponent is tried first, so "1E3" is 1000, not 1 exa
// followed by garbage. Returns null for anything outside the grammar.
const QUANTITY_PATTERN = /^([+-]?(?:\d+\.?\d*|\.\d+))(?:([KMGTPE]i)|[eE]([+-]?\d+)|([numkMGTPE]?))$/;

// Returns an error message when value is not a Kubernetes quantity, else null.
export function quantityError(value) {
  return QUANTITY_PATTERN.test(value) ? null : 'Use a quantity like 500Mi or 10Gi.';
}

// Whole bytes, rounded up like Quantity.Value().
function parseQuantity(quantity) {
  const match = QUANTITY_PATTERN.exec(quantity);
  if (!match) return null;
  const [, number, binarySI, exponent, decimalSI] = match;
  const value = exponent !== undefined
    ? Number(number) * 10 ** Number(exponent)
    : Number(number) * QUANTITY_MULTIPLIERS[binarySI ?? decimalSI];
  return Math.ceil(value);
}

// The console's memory format (RegionCatalogService.formatMemory): truncated
// to one decimal, without a trailing ".0". Storage adds TiB and PiB above
// 1024 of the unit below.
const SIZE_UNITS = [['PiB', 2 ** 50], ['TiB', 2 ** 40], ['GiB', 2 ** 30]];

function formatSize(bytes) {
  const [unit, size] = SIZE_UNITS.find(([, s]) => bytes >= s) ?? SIZE_UNITS.at(-1);
  const tenths = Math.floor((bytes * 10) / size);
  const whole = Math.floor(tenths / 10);
  const frac = tenths % 10;
  return frac === 0 ? `${whole} ${unit}` : `${whole}.${frac} ${unit}`;
}

// Renders a byte Quantity ("20478Mi") as "19.9 GiB". A value outside the
// Quantity grammar is shown as written rather than as a misleading 0 GiB.
// Raw capacity from the CephCluster's own status (status.ceph.capacity);
// null when no cluster reports one or the viewer may not read it.
export async function fetchCephCapacity() {
  try {
    const { items } = await fundament.k8s.list({ group: 'ceph.rook.io', version: 'v1', resource: 'cephclusters' });
    const cap = items?.[0]?.status?.ceph?.capacity;
    return cap?.bytesTotal ? cap : null;
  } catch {
    return null;
  }
}

export function humanizeQuantity(quantity) {
  if (quantity === undefined || quantity === null || quantity === '') return '—';
  const bytes = parseQuantity(String(quantity).trim());
  if (bytes === null) return String(quantity);
  const formatted = formatSize(Math.abs(bytes));
  return bytes < 0 && formatted !== '0 GiB' ? `-${formatted}` : formatted;
}
