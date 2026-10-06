// Buckets section for the ObjectStorage detail page: lists the
// ObjectBucketClaims provisioned against one store's StorageClass, with
// inline per-claim detail and an inline create form. Claims are namespaced
// and live wherever workloads do, so the list queries cluster-wide and
// shows the namespace — per-project-namespace scoping would hide claims in
// namespaces the project does not own (on a dev sandbox, all of them).
//
// Mounted by consumer-pages.js via the OBJECTSTORAGE config's detailSection
// hook; the SDK is already loaded and initialized by then.

import {
  ensureNldd,
  showSheetError,
  openSheet,
  quantityError,
  escapeHtml,
  emptyRow,
  errorRow,
  renderDefList,
  resourceNameError,
  wireSubmit,
} from './_shared.js';

const OBC = { group: 'objectbucket.io', version: 'v1alpha1', resource: 'objectbucketclaims' };
const OBJECTBUCKETS = { group: 'objectbucket.io', version: 'v1alpha1', resource: 'objectbuckets' };
const CONFIGMAPS = { group: '', version: 'v1', resource: 'configmaps' };

// requestedBucket is the claim's own naming request; a Bound claim's real
// name lives in its ConfigMap, which only the per-claim view fetches.
export function requestedBucket(spec) {
  if (spec?.bucketName) return spec.bucketName;
  if (spec?.generateBucketName) return `${spec.generateBucketName}-…`;
  return '—';
}

// S3 bucket names: 3-63 chars, lowercase letters, digits and dashes, starting
// and ending alphanumeric. Dots are legal in S3 but break TLS and path-style
// assumptions, so the form does not offer them. Returns an error or null.
export function bucketNameError(name) {
  if (name.length < 3 || name.length > 63) {
    return 'Bucket names must be 3 to 63 characters.';
  }
  if (!/^[a-z0-9][a-z0-9-]*[a-z0-9]$/.test(name)) {
    return 'Bucket names use lowercase letters, digits and dashes, starting and ending with a letter or digit.';
  }
  return null;
}

export function mountBucketsSection(container, item, ctx, cfg) {
  const storageClassName = item.status?.storageClassName ?? `${cfg.storageClassPrefix}${item.metadata?.name}`;
  const namespaces = ctx.namespaces ?? [];

  container.innerHTML = `
    <h2 class="plugin-heading">Buckets</h2>
    <div class="buckets-body"></div>
  `;
  const body = container.querySelector('.buckets-body');

  showList();

  async function showList() {
    const headers = ['Name', 'Namespace', 'Phase', 'Bucket'];
    body.innerHTML = `
      <div class="plugin-actions">
        <button type="button" class="plugin-button" data-role="create">Create bucket</button>
      </div>
      <table class="plugin-table">
        <thead><tr>${headers.map((h) => `<th>${escapeHtml(h)}</th>`).join('')}</tr></thead>
        <tbody>${emptyRow(headers.length, 'Loading…')}</tbody>
      </table>
    `;
    body.querySelector('[data-role="create"]').addEventListener('click', async () => {
      try {
        await ensureNldd();
      } catch (err) {
        showSheetError(err);
        return;
      }
      showCreate();
    });
    const tbody = body.querySelector('tbody');

    let claims;
    try {
      const { items } = await fundament.k8s.list(OBC);
      claims = (items ?? []).filter((c) => c.spec?.storageClassName === storageClassName);
    } catch (err) {
      tbody.innerHTML = errorRow(headers.length, err);
      return;
    }

    if (claims.length === 0) {
      tbody.innerHTML = emptyRow(headers.length, 'No buckets on this storage yet.');
      return;
    }
    tbody.innerHTML = claims
      .map((c) => `
        <tr data-name="${escapeHtml(c.metadata?.name ?? '')}" data-namespace="${escapeHtml(c.metadata?.namespace ?? '')}">
          <td><a href="#" class="row-link">${escapeHtml(c.metadata?.name ?? '')}</a></td>
          <td>${escapeHtml(c.metadata?.namespace ?? '')}</td>
          <td>${escapeHtml(c.status?.phase ?? 'Unknown')}</td>
          <td>${escapeHtml(requestedBucket(c.spec))}</td>
        </tr>`)
      .join('');
    tbody.querySelectorAll('a.row-link').forEach((link) => {
      link.addEventListener('click', async (e) => {
        e.preventDefault();
        const row = link.closest('tr');
        try {
          await ensureNldd();
        } catch (err) {
          showSheetError(err);
          return;
        }
        showBucket(row.dataset.name, row.dataset.namespace);
      });
    });
  }

  async function showBucket(name, namespace) {
    const { body: sheetBody } = openSheet({ label: `Bucket · ${name}` });
    sheetBody.insertAdjacentHTML('beforeend', '<p class="plugin-text">Loading…</p>');
    try {
      const claim = await fundament.k8s.get({ ...OBC, namespace, name });
      const spec = claim.spec ?? {};
      const pairs = [
        ['Phase', claim.status?.phase ?? 'Unknown'],
        ['Namespace', namespace],
        [spec.bucketName ? 'Requested bucket name' : 'Bucket name prefix',
          spec.bucketName ?? spec.generateBucketName ?? '—'],
      ];
      if (spec.additionalConfig?.maxSize) pairs.push(['Max size', spec.additionalConfig.maxSize]);
      if (spec.additionalConfig?.maxObjects) pairs.push(['Max objects', spec.additionalConfig.maxObjects]);

      // The connection ConfigMap appears when the claim binds; before that
      // its absence is the expected state, not an error.
      let connection = '<p class="plugin-text">Connection details appear once the claim is Bound.</p>';
      try {
        const cm = await fundament.k8s.get({ ...CONFIGMAPS, namespace, name });
        connection = renderDefList([
          ['Endpoint', `http://${cm.data?.BUCKET_HOST ?? '?'}:${cm.data?.BUCKET_PORT ?? '?'}`],
          ['Bucket', cm.data?.BUCKET_NAME ?? '—'],
        ]);
      } catch {
        // keep the placeholder
      }

      // The Secret's name is shown; its values never reach this iframe.
      const mount = `envFrom:
  - configMapRef: { name: ${name} }
  - secretRef: { name: ${name} }`;
      sheetBody.lastElementChild.outerHTML = `
        ${renderDefList(pairs)}
        ${connection}
        <p class="plugin-hint">
          Credentials are in the Secret <code>${escapeHtml(name)}</code> (same namespace),
          as AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY. Give a pod both with:
        </p>
        <pre class="plugin-text"><code>${escapeHtml(mount)}</code></pre>
      `;
    } catch (err) {
      sheetBody.lastElementChild.outerHTML = `<div class="plugin-error">${escapeHtml(`Failed to load: ${err?.message ?? err}`)}</div>`;
    }
  }

  function showCreate() {
    const { body: sheetBody, close } = openSheet({ label: 'Create Bucket' });
    // A project carries its namespaces; without any (dev sandbox), the
    // namespace is free text instead of a dropdown.
    const namespaceControl = namespaces.length > 0
      ? `<select id="bucket-namespace" name="namespace" class="plugin-select">
          ${namespaces.map((ns) => `<option value="${escapeHtml(ns)}">${escapeHtml(ns)}</option>`).join('')}
        </select>`
      : `<input id="bucket-namespace" name="namespace" type="text" class="plugin-input" placeholder="default" />`;

    sheetBody.insertAdjacentHTML('beforeend', `
      <p class="plugin-text">
        The bucket is provisioned on this storage (class
        <code>${escapeHtml(storageClassName)}</code>). The endpoint lands in a
        ConfigMap and the credentials in a Secret, both named after the claim,
        in the claim's namespace.
      </p>
      <form class="plugin-form" novalidate>
        <div class="plugin-error" data-role="error" hidden></div>

        <div class="plugin-field">
          <label class="plugin-label" for="bucket-namespace">Namespace</label>
          ${namespaceControl}
        </div>

        <div class="plugin-field">
          <label class="plugin-label" for="claim-name">Name</label>
          <input id="claim-name" name="name" type="text" class="plugin-input"
                 placeholder="my-bucket" required
                 pattern="[a-z0-9]([a-z0-9\\-]*[a-z0-9])?" maxlength="63" />
          <span class="plugin-hint">Names the claim and its ConfigMap/Secret. Lowercase letters, digits and dashes.</span>
        </div>

        <div class="plugin-field">
          <label class="plugin-label" for="naming-mode">Bucket name</label>
          <select id="naming-mode" name="namingMode" class="plugin-select">
            <option value="generate" selected>Generate from the claim name (recommended)</option>
            <option value="exact">Exact name</option>
          </select>
          <span class="plugin-hint">Generated names get a random suffix, so they cannot collide.</span>
        </div>

        <div class="plugin-field" data-role="exact-name" hidden>
          <label class="plugin-label" for="bucket-name">Exact bucket name</label>
          <input id="bucket-name" name="bucketName" type="text" class="plugin-input" maxlength="63" />
          <span class="plugin-hint">Bucket names are shared across the whole object store. Names taken outside Kubernetes are only caught by the provisioner: the claim then stays Pending.</span>
        </div>

        <div class="plugin-field">
          <label class="plugin-label" for="max-size">Max size (optional)</label>
          <input id="max-size" name="maxSize" type="text" class="plugin-input" placeholder="10Gi" />
          <span class="plugin-hint">Quota for the bucket, e.g. 10Gi. Empty means unlimited.</span>
        </div>

        <div class="plugin-field">
          <label class="plugin-label" for="max-objects">Max objects (optional)</label>
          <input id="max-objects" name="maxObjects" type="number" class="plugin-input" min="1" />
          <span class="plugin-hint">Empty means unlimited.</span>
        </div>

        <div class="plugin-actions">
          <button type="submit" class="plugin-button" data-role="submit">Create bucket</button>
          <button type="button" class="plugin-button-secondary" data-role="cancel">Cancel</button>
        </div>
      </form>
    `);

    const form = sheetBody.querySelector('form');
    const nameInput = form.querySelector('[name="name"]');
    const modeSelect = form.querySelector('[name="namingMode"]');
    const exactField = sheetBody.querySelector('[data-role="exact-name"]');
    modeSelect.addEventListener('change', () => {
      exactField.hidden = modeSelect.value !== 'exact';
    });
    sheetBody.querySelector('[data-role="cancel"]').addEventListener('click', () => close());

    wireSubmit(form, {
      button: sheetBody.querySelector('[data-role="submit"]'),
      errorBox: sheetBody.querySelector('[data-role="error"]'),
      busyLabel: 'Creating…',
      failPrefix: 'Failed to create bucket',
      validate: () => {
        if (!form.querySelector('[name="namespace"]').value.trim()) {
          return 'Please choose a namespace.';
        }
        const invalid = resourceNameError(nameInput.value.trim(), 63);
        if (invalid) {
          nameInput.focus();
          return invalid;
        }
        if (modeSelect.value === 'exact') {
          const bad = bucketNameError(form.querySelector('[name="bucketName"]').value.trim());
          if (bad) return bad;
        }
        const maxSize = form.querySelector('[name="maxSize"]').value.trim();
        if (maxSize && quantityError(maxSize)) {
          return `Max size: ${quantityError(maxSize)}`;
        }
        const maxObjects = form.querySelector('[name="maxObjects"]').value.trim();
        if (maxObjects && (!/^[0-9]+$/.test(maxObjects) || Number(maxObjects) < 1)) {
          return 'Max objects must be a whole number of at least 1.';
        }
        return null;
      },
      action: async () => {
        const name = nameInput.value.trim();
        const namespace = form.querySelector('[name="namespace"]').value.trim();
        const spec = { storageClassName };

        if (modeSelect.value === 'exact') {
          const bucketName = form.querySelector('[name="bucketName"]').value.trim();
          // Collision pre-check against the cluster's bucket inventory.
          // Names taken outside Kubernetes are not in it; the provisioner is
          // the real referee and leaves such a claim Pending.
          const { items } = await fundament.k8s.list(OBJECTBUCKETS);
          // Scoped to this store's class: RGW bucket names are per-store, so
          // a name on another ObjectStorage is no collision here.
          const taken = (items ?? []).some((ob) =>
            ob.spec?.storageClassName === storageClassName
            && ob.spec?.endpoint?.bucketName === bucketName);
          if (taken) throw Error(`bucket name "${bucketName}" is already in use`);
          spec.bucketName = bucketName;
        } else {
          spec.generateBucketName = name;
        }

        // additionalConfig values are strings (the field is map[string]string).
        const maxSize = form.querySelector('[name="maxSize"]').value.trim();
        const maxObjects = form.querySelector('[name="maxObjects"]').value.trim();
        if (maxSize || maxObjects) {
          spec.additionalConfig = {};
          if (maxSize) spec.additionalConfig.maxSize = maxSize;
          if (maxObjects) spec.additionalConfig.maxObjects = maxObjects;
        }

        await fundament.k8s.create(
          { ...OBC, namespace },
          {
            apiVersion: 'objectbucket.io/v1alpha1',
            kind: 'ObjectBucketClaim',
            metadata: { name, namespace },
            spec,
          },
        );
        close();
        // The new claim appears in the refreshed list; its sheet opens on top.
        showList();
        await showBucket(name, namespace);
      },
    });
  }
}
