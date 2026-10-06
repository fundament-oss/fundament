// Shared helpers for ceph-rook plugin templates. Plain ES module so templates
// can `import` from it.
//
// The plugin CSP has no 'unsafe-inline', which rules out onclick= and inline
// style alike: events go through addEventListener, styling through the
// .plugin-* classes in plugin-sdk.css.

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

// Loads the shared NLDD Design System bundle the console serves next to the
// SDK (FUN-18), registering the <nldd-*> elements, and mirrors the SDK's
// light/dark body class into the data-scheme attribute the components read.
// Memoized and called lazily from the sheet-opening handlers, so a host
// without the bundle still serves the read-only views. Both halves are
// awaited: an unawaited stylesheet would render sheets unstyled and swallow
// the error.
let nlddLoad;
export function ensureNldd() {
  nlddLoad ??= (() => {
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
  })();
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
// <dialog>; inside the content flow it would steal layout height). Returns
// the content container and a close(); the element removes itself after the
// closing animation. Requires ensureNldd() to have resolved.
//
// The host sizes the iframe to this document's height, so on a short page
// the sheet — confined to the iframe's viewport — would render a few rows
// tall. Growing the body while the sheet is open makes the host grow the
// iframe; newer hosts also floor the iframe at the viewport remainder, which
// usually dominates this. CSSOM property assignment, not style attributes:
// the CSP blocks only the latter.
let openSheetCount = 0;
let preSheetMinHeight = '';

export function openSheet({ label, width = '480px', minHeight = '640px' }) {
  // Counted, not saved per sheet: chained sheets (close one, open the next
  // in the same tick) would capture the inflated value as "previous" and
  // latch it; only the outermost open/last close touch the body.
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
    const heading = document.createElement('h2');
    heading.className = 'plugin-heading';
    heading.textContent = label;
    body.appendChild(heading);
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

// Replicas <select> shared by every consumer create/edit form. "auto" stands
// for an absent spec.replicas.
export function replicasFieldHtml(replicas) {
  const selected = replicas ? String(replicas) : 'auto';
  const label = (v) => (v === 'auto' ? 'auto (recommended)' : {
    1: '1 — no replication',
    2: '2 — two replicas',
    3: '3 — three replicas',
  }[v]);
  const options = ['auto', '1', '2', '3']
    .map((v) => `<option value="${v}"${v === selected ? ' selected' : ''}>${label(v)}</option>`)
    .join('');
  return `
    <div class="plugin-field">
      <label class="plugin-label" for="replicas">Replicas</label>
      <select id="replicas" name="replicas" class="plugin-select">${options}</select>
      <span class="plugin-hint">auto derives the replica count from the number of nodes contributing disks to the cluster.</span>
    </div>`;
}

// Reads the replicas <select> into spec.replicas. null, not undefined, so a
// merge-patch removes the field when the operator switches back to auto.
export function replicasValue(form) {
  const value = form.querySelector('[name="replicas"]').value;
  return value === 'auto' ? null : Number(value);
}

// Default-StorageClass checkbox shared by the BlockStorage create and edit
// forms. At most one BlockStorage may set it; conflicts show as Degraded.
export function defaultFieldHtml(checked = false) {
  return `
    <div class="plugin-field">
      <label class="plugin-checkbox">
        <input type="checkbox" name="default"${checked ? ' checked' : ''} />
        Default StorageClass
      </label>
      <span class="plugin-hint">PersistentVolumeClaims without an explicit storageClassName use this class. Only one BlockStorage may be the default; a second one degrades both until resolved.</span>
    </div>`;
}

// One integer input field, driven by a field descriptor (see the intFields
// descriptors in consumer-pages.js), so each field's CRD bounds live in one
// place instead of a copy per form and per validator.
export function integerFieldHtml(field, value) {
  return `
    <div class="plugin-field">
      <label class="plugin-label" for="${field.id}">${escapeHtml(field.label)}</label>
      <input id="${field.id}" name="${field.name}" type="number" class="plugin-input"
             min="${field.min}" max="${field.max}" value="${escapeHtml(String(value))}" />
      <span class="plugin-hint">${escapeHtml(field.hint)}</span>
    </div>`;
}

// Bounds mirror the field's CRD validation; returns an error message or null.
export function integerFieldError(field, value) {
  if (!Number.isInteger(value) || value < field.min || value > field.max) {
    return `${field.label} must be a whole number from ${field.min} to ${field.max}.`;
  }
  return null;
}

// Wires the shared submit flow: validate, disable the button with busyLabel,
// run action, surface failures in errorBox and restore the button. On success
// the button stays disabled -- the action navigates or re-renders.
export function wireSubmit(form, { button, errorBox, busyLabel, failPrefix, validate, action }) {
  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    errorBox.hidden = true;

    // validate runs inside the try: preventDefault() already suppressed the
    // native submit, so a validator that throws would otherwise leave the
    // form silently dead — no error box, no native validation, nothing.
    const idleLabel = button.textContent;
    try {
      const invalid = validate?.();
      if (invalid) {
        errorBox.textContent = invalid;
        errorBox.hidden = false;
        return;
      }
      button.disabled = true;
      button.textContent = busyLabel;
      await action();
    } catch (err) {
      errorBox.textContent = `${failPrefix}: ${err?.message ?? err}`;
      errorBox.hidden = false;
      button.disabled = false;
      button.textContent = idleLabel;
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
export function humanizeQuantity(quantity) {
  if (quantity === undefined || quantity === null || quantity === '') return '—';
  const bytes = parseQuantity(String(quantity).trim());
  if (bytes === null) return String(quantity);
  const formatted = formatSize(Math.abs(bytes));
  return bytes < 0 && formatted !== '0 GiB' ? `-${formatted}` : formatted;
}
