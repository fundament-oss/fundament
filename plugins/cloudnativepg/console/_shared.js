// Shared helpers for the cloudnativepg plugin pages. Plain ES module so pages
// can `import` from it. Trimmed from ceph-rook's _shared.js.
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

// The element that carries `invalid` for a field: the <nldd-dropdown> around a
// <select>, or the control itself.
function fieldControl(el) {
  return el.closest('nldd-dropdown') ?? el;
}

// Shows message in the field's nldd-validation-item, the one rule-less item in
// its nldd-validation-list, and marks the field invalid.
function setFieldError(el, message) {
  const control = fieldControl(el);
  const item = control.closest('nldd-form-field')?.querySelector('nldd-validation-item');
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
// returns an error message or null; each failure shows under its own field and
// the first failing field takes focus. A failing action shows in the critical
// errorBanner. On success the button keeps loading: the action navigates away.
export function wireSubmit(form, { button, errorBanner, failPrefix, checks, action }) {
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
      const message = check(control.value.trim());
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
