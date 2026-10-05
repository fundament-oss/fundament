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

// Asks the host for this kind's create route. A custom list UI needs its own
// "Add": the console only renders its built-in Create button for kinds without a
// custom list component, so without this the create view is unreachable.
export function navigateToCreate() {
  window.parent.postMessage(
    { type: 'plugin:create' },
    window.fundament?.parentOrigin ?? '*',
  );
}

// Returns to the resource-kind list. Only meaningful from a create or detail
// view; the host ignores it on a list.
export function navigateBack() {
  window.parent.postMessage(
    { type: 'plugin:navigate-back' },
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

// Metadata-servers input shared by the FileStorage create and edit forms.
export function metadataServersFieldHtml(value = 1) {
  return `
    <div class="plugin-field">
      <label class="plugin-label" for="mds-count">Metadata servers</label>
      <input id="mds-count" name="metadataServers" type="number" class="plugin-input"
             min="1" max="5" value="${escapeHtml(String(value))}" />
      <span class="plugin-hint">Active MDS daemons; each gets a standby. 1 is right unless metadata throughput at scale demands more.</span>
    </div>`;
}

// Bounds mirror the CRD's validation; returns an error message or null.
export function metadataServersError(value) {
  if (!Number.isInteger(value) || value < 1 || value > 5) {
    return 'Metadata servers must be a whole number from 1 to 5.';
  }
  return null;
}

// Gateway-instances input shared by the ObjectStorage create and edit forms.
export function gatewayInstancesFieldHtml(value = 1) {
  return `
    <div class="plugin-field">
      <label class="plugin-label" for="rgw-count">Gateway instances</label>
      <input id="rgw-count" name="gatewayInstances" type="number" class="plugin-input"
             min="1" max="5" value="${escapeHtml(String(value))}" />
      <span class="plugin-hint">RGW pods serving the S3 API. 1 is right unless request throughput demands more.</span>
    </div>`;
}

// Bounds mirror the CRD's validation; returns an error message or null.
export function gatewayInstancesError(value) {
  if (!Number.isInteger(value) || value < 1 || value > 5) {
    return 'Gateway instances must be a whole number from 1 to 5.';
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

    const invalid = validate?.();
    if (invalid) {
      errorBox.textContent = invalid;
      errorBox.hidden = false;
      return;
    }

    const idleLabel = button.textContent;
    button.disabled = true;
    button.textContent = busyLabel;
    try {
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
