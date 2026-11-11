// Shared helpers for ceph-rook plugin templates. Plain ES module so templates
// can `import` from it.
//
// The plugin CSP has no 'unsafe-inline', which rules out onclick= and inline
// style alike: events go through addEventListener, styling through the
// <nldd-*> components, which every page loads (bootPage).
//
// The Console draws each page's title bar, title and way back, so a
// page starts with its own content: no <h1>, no back button.

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
// Memoized; the stylesheet is awaited too, or a page renders unstyled with the
// error swallowed.
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

// Every page starts here: the SDK and the design system side by side, then
// the init context. The NLDD bundle is what sets the pages in RijksSans and
// draws every <nldd-*> element, so a page does not render without it.
export async function bootPage() {
  try {
    await Promise.all([loadSdk(), ensureNldd()]);
  } catch (err) {
    // Without the bundle no <nldd-*> element renders, an error banner
    // included, so this one says it in plain text.
    const message = document.createElement('p');
    message.className = 'plugin-text';
    message.textContent = `Cannot open this page: ${err?.message ?? err}`;
    document.querySelector('.plugin-card')?.replaceChildren(message);
    throw err;
  }
  return fundament.init;
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

// A critical banner for a page or section whose data failed to load.
export function errorBox(err, prefix = 'Failed to load') {
  return `<nldd-banner variant="critical" text="${escapeHtml(`${prefix}: ${err?.message ?? err}`)}"></nldd-banner>`;
}

// What a list or detail shows while it reads.
export function loadingHtml(text = 'Loading…') {
  return `<nldd-inline-dialog variant="loading" text="${escapeHtml(text)}"></nldd-inline-dialog>`;
}

// A list with nothing in it: what is missing, and what makes it appear.
export function emptyStateHtml({ icon, text, supportingText }) {
  return `<nldd-inline-dialog icon="${escapeHtml(icon)}" icon-color="secondary" text="${escapeHtml(text)}"${
    supportingText ? ` supporting-text="${escapeHtml(supportingText)}"` : ''
  }></nldd-inline-dialog>`;
}

// The badge colors definition.yaml's uiHints give the Console's own views,
// plus a claim's Bound/Pending/Lost. Anything else is said as is, in gray.
const PHASE_COLORS = {
  Ready: 'success',
  Bound: 'success',
  Provisioning: 'warning',
  Pending: 'warning',
  Degraded: 'critical',
  Lost: 'critical',
};

export function phaseBadgeHtml(phase) {
  const text = phase || 'Unknown';
  return `<nldd-badge color="${PHASE_COLORS[text] ?? 'neutral'}" text="${escapeHtml(text)}"></nldd-badge>`;
}

// A list of resources as one box of rows, each a button that opens the
// resource: its name, a summary under it, and its status at the end. The
// summary carries what the old table columns did, as a sentence rather than a
// column that is empty for most rows.
export function resourceListHtml(rows, accessibleLabel) {
  const items = rows
    .map(({ name, namespace, text, supportingText, badge }) => {
      const ns = namespace ? ` data-namespace="${escapeHtml(namespace)}"` : '';
      const supporting = supportingText ? ` supporting-text="${escapeHtml(supportingText)}"` : '';
      const status = badge
        ? `<nldd-spacer-cell size="12"></nldd-spacer-cell><nldd-cell width="fit-content">${badge}</nldd-cell>`
        : '';
      return `
        <nldd-list-item button data-name="${escapeHtml(name)}"${ns}>
          <nldd-text-cell text="${escapeHtml(text ?? name)}"${supporting}></nldd-text-cell>
          ${status}
          <nldd-spacer-cell size="8"></nldd-spacer-cell>
          <nldd-icon-cell icon="chevron-right" color="secondary" size="20"></nldd-icon-cell>
        </nldd-list-item>`;
    })
    .join('');
  return `<nldd-list appearance="box-tinted" accessible-label="${escapeHtml(accessibleLabel)}">${items}</nldd-list>`;
}

// Opens a row's resource on click, and on Enter or Space, which the row's
// inner button turns into a click.
export function wireListRows(root, open) {
  root.querySelectorAll('nldd-list-item[data-name]').forEach((row) => {
    row.addEventListener('click', () => open(row.dataset.name, row.dataset.namespace || undefined));
  });
}

// A titled box on a detail page, as the Console's generated detail page draws
// its sections.
export function sectionHtml(title, body) {
  return `
    <nldd-box>
      <nldd-container padding="16" md-padding="24">
        <nldd-title size="5"><h2>${escapeHtml(title)}</h2></nldd-title>
        <nldd-spacer size="16"></nldd-spacer>
        ${body}
      </nldd-container>
    </nldd-box>`;
}

// Secondary explanation under a list or a section. Trusted markup: callers
// escape what they put in.
export function hintHtml(html) {
  return `<nldd-text size="sm" color="secondary">${html}</nldd-text>`;
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

// Renders [key, value] pairs as an nldd-list, label left and value right: the
// same rows the Console's generated detail page draws. Both halves are escaped
// here, so pass raw values: pre-escaped ones render as "&amp;".
export function renderKeyValueList(pairs, accessibleLabel) {
  const rows = pairs
    .map(
      ([k, v]) => `
        <nldd-list-item>
          <nldd-text-cell text="${escapeHtml(k)}"></nldd-text-cell>
          <nldd-spacer-cell size="12"></nldd-spacer-cell>
          <nldd-text-cell width="fit-content" horizontal-alignment="right" text="${escapeHtml(v)}"></nldd-text-cell>
        </nldd-list-item>`,
    )
    .join('');
  return `<nldd-list appearance="simple" accessible-label="${escapeHtml(accessibleLabel)}">${rows}</nldd-list>`;
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

// The Save/Create and Cancel pair at the bottom of every form, in the same
// group the Console's own dialogs use. The spacer keeps the buttons off the
// bottom edge of the sheet.
export function formActionsHtml({ submitId, submitText, cancelId }) {
  return `
    <nldd-form-actions>
      <nldd-button-group orientation="horizontal">
        <nldd-button id="${submitId}" type="submit" appearance="primary" text="${escapeHtml(submitText)}"></nldd-button>
        <nldd-button id="${cancelId}" type="button" appearance="secondary" text="Cancel"></nldd-button>
      </nldd-button-group>
    </nldd-form-actions>
    <nldd-spacer size="24"></nldd-spacer>`;
}

export function errorBannerHtml(id) {
  return `<nldd-banner id="${id}" variant="critical" hidden></nldd-banner>`;
}

// A standalone critical banner, for a sheet page whose data failed to load.
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
// keeps loading: the action closes the sheet or navigates.
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

// Whole bytes, rounded up like Quantity.Value(). null for anything outside the
// grammar, and for an exponent too large for a number: "0e999" would
// otherwise be NaN.
function parseQuantity(quantity) {
  const match = QUANTITY_PATTERN.exec(quantity);
  if (!match) return null;
  const [, number, binarySI, exponent, decimalSI] = match;
  const value = exponent !== undefined
    ? Number(number) * 10 ** Number(exponent)
    : Number(number) * QUANTITY_MULTIPLIERS[binarySI ?? decimalSI];
  return Number.isFinite(value) ? Math.ceil(value) : null;
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
