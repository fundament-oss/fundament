// Shared helpers for ceph-rook plugin templates. Plain ES module so templates
// can `import` from it.
//
// The plugin CSP has no 'unsafe-inline', which rules out onclick= and inline
// style alike: events go through addEventListener, styling through the
// <nldd-*> components and the .plugin-* classes in plugin-sdk.css.

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

// Loads the NLDD Design System bundle the Console serves next to the SDK, which
// registers every <nldd-*> element. Resolves once both the stylesheet and the
// script are in, so no component renders unstyled.
export function loadNlddDesignSystem() {
  // The design system reads light/dark from :root[data-scheme]; mirror the
  // .light/.dark class the SDK sets on <body> on init and on every theme change.
  const syncScheme = () => {
    const dark = document.body.classList.contains('dark');
    document.documentElement.setAttribute('data-scheme', dark ? 'dark' : 'light');
  };
  syncScheme();
  new MutationObserver(syncScheme).observe(document.body, {
    attributes: true,
    attributeFilter: ['class'],
  });

  const settled = (el, what) =>
    new Promise((resolve, reject) => {
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

  return Promise.all([css, js]);
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

// A standalone critical banner, for a page that failed to load.
export function loadErrorBanner(message) {
  const banner = document.createElement('nldd-banner');
  banner.setAttribute('variant', 'critical');
  banner.setAttribute('text', message);
  return banner;
}

// Replication dropdown shared by every consumer create/edit form.
export function replicationFieldHtml(selected = 'auto') {
  const label = (v) => (v === 'auto' ? 'auto (recommended)' : {
    1: '1 (no replication)',
    2: '2 replicas',
    3: '3 replicas',
  }[v]);
  const options = ['auto', '1', '2', '3']
    .map((v) => `<option value="${v}"${v === selected ? ' selected' : ''}>${label(v)}</option>`)
    .join('');
  return formFieldHtml(
    'Replication',
    `<nldd-dropdown><select id="replication" name="replication">${options}</select></nldd-dropdown>`,
    { hint: 'auto derives the replica count from the number of nodes contributing disks to the cluster.' },
  );
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

// Metadata-servers field shared by the FileStorage create and edit forms.
export function metadataServersFieldHtml(value = 1) {
  return formFieldHtml(
    'Metadata servers',
    `<nldd-number-field id="mds-count" name="metadataServers" min="1" max="5"
                        value="${escapeHtml(String(value))}" width="160px"></nldd-number-field>`,
    {
      errorId: 'mds-count-error',
      hint: 'Active MDS daemons; each gets a standby. 1 is right unless metadata throughput at scale demands more.',
    },
  );
}

// Bounds mirror the CRD's validation; returns an error message or null.
export function metadataServersError(value) {
  if (!Number.isInteger(value) || value < 1 || value > 5) {
    return 'Metadata servers must be a whole number from 1 to 5.';
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
// keeps loading: the action navigates or re-renders.
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
