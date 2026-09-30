// Shared helpers for the cloudnativepg plugin pages. Plain ES module so pages
// can `import` from it. Trimmed from ceph-rook's _shared.js.
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

// Posts a navigate message to the parent, which resolves it relative to the
// iframe's current route, so this sends only the resource identity and works
// from a create view too. parentOrigin falls back to '*' before init.
export function navigateToDetail(name, namespace) {
  window.parent.postMessage(
    { type: 'plugin:navigate', name, namespace },
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

// Renders a key/value definition list from [key, value] pairs. Both halves are
// escaped here, so pass raw values: pre-escaped ones render as "&amp;".
export function renderDefList(pairs) {
  const rows = pairs
    .map(([k, v]) => `<dt>${escapeHtml(k)}</dt><dd>${escapeHtml(v)}</dd>`)
    .join('');
  return `<dl class="plugin-deflist">${rows}</dl>`;
}

// Wires the shared submit flow: validate, disable the button with busyLabel,
// run action, surface failures in errorBox and restore the button. On success
// the button stays disabled: the action navigates away.
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
