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
// error message or null.
export function resourceNameError(name) {
  if (!name) return 'Please enter a name.';
  if (!/^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/.test(name)) {
    return 'Use lowercase letters, digits and dashes only.';
  }
  return null;
}

// Replication <select> shared by every consumer create/edit form.
export function replicationFieldHtml(selected = 'auto') {
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
      <label class="plugin-label" for="replication">Replication</label>
      <select id="replication" name="replication" class="plugin-select">${options}</select>
      <span class="plugin-hint">auto derives the replica count from the number of nodes contributing disks to the cluster.</span>
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

export function humanizeBytes(bytes) {
  if (!bytes || bytes === 0) return '0 B';
  const tib = bytes / 1024 ** 4;
  if (tib >= 1) return `${tib.toFixed(1)} TiB`;
  const gib = bytes / 1024 ** 3;
  if (gib >= 1) return `${gib.toFixed(1)} GiB`;
  const mib = bytes / 1024 ** 2;
  if (mib >= 1) return `${mib.toFixed(0)} MiB`;
  return `${bytes} B`;
}
