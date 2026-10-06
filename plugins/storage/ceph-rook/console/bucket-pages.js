// Buckets section for the ObjectStorage detail page: the ObjectBucketClaims
// provisioned against one store's StorageClass, with per-claim detail and a
// create form in sheets. Claims live wherever workloads do, so the list
// queries cluster-wide; per-project-namespace scoping would hide them.
// Mounted via the OBJECTSTORAGE detailSection hook, after the SDK loaded.

import {
  openSheet,
  errorBox,
  quantityError,
  escapeHtml,
  emptyRow,
  errorRow,
  renderDefList,
  resourceNameError,
  wireSubmit,
} from './_shared.js';

const OBC = { group: 'objectbucket.io', version: 'v1alpha1', resource: 'objectbucketclaims' };
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
    body.querySelector('[data-role="create"]').addEventListener('click', () => showCreate());
    const tbody = body.querySelector('tbody');

    let claims;
    let failedNamespaces = [];
    try {
      // Project members may only list in their namespaces; without project
      // namespaces (org admin, dev sandbox) one cluster-wide list works.
      if (namespaces.length > 0) {
        const results = await Promise.allSettled(
          namespaces.map((ns) => fundament.k8s.list({ ...OBC, namespace: ns })),
        );
        claims = [];
        results.forEach((res, i) => {
          if (res.status === 'fulfilled') claims.push(...(res.value.items ?? []));
          else failedNamespaces.push(namespaces[i]);
        });
        if (failedNamespaces.length === results.length) throw results[0].reason;
      } else {
        const { items } = await fundament.k8s.list(OBC);
        claims = items ?? [];
      }
      claims = claims.filter((c) => c.spec?.storageClassName === storageClassName);
    } catch (err) {
      tbody.innerHTML = errorRow(headers.length, err);
      return;
    }

    const failureNote = failedNamespaces.length === 0 ? '' : errorRow(
      headers.length, Error(`listing failed in ${failedNamespaces.join(', ')}`));
    if (claims.length === 0) {
      tbody.innerHTML = failureNote || emptyRow(headers.length, 'No buckets on this storage yet.');
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
      .join('') + failureNote;
    tbody.querySelectorAll('a.row-link').forEach((link) => {
      link.addEventListener('click', (e) => {
        e.preventDefault();
        const row = link.closest('tr');
        showBucket(row.dataset.name, row.dataset.namespace);
      });
    });
  }

  // preloaded carries the object a create just returned: it is never Bound
  // yet, so both the claim get and the ConfigMap get would be wasted.
  async function showBucket(name, namespace, preloaded) {
    const sheet = await openSheet({ label: `Bucket · ${name}` });
    if (!sheet) return;
    const sheetBody = sheet.body;
    const closeSheet = sheet.close;
    sheetBody.insertAdjacentHTML('beforeend', '<p class="plugin-text">Loading…</p>');
    try {
      const [claim, cm] = preloaded
        ? [preloaded, undefined]
        : await Promise.all([
            fundament.k8s.get({ ...OBC, namespace, name }),
            // The connection ConfigMap only appears once the claim binds.
            fundament.k8s.get({ ...CONFIGMAPS, namespace, name }).catch(() => undefined),
          ]);
      const spec = claim.spec ?? {};
      const pairs = [
        ['Phase', claim.status?.phase ?? 'Unknown'],
        ['Namespace', namespace],
        [spec.bucketName ? 'Requested bucket name' : 'Bucket name prefix',
          spec.bucketName ?? spec.generateBucketName ?? '—'],
      ];
      if (spec.additionalConfig?.maxSize) pairs.push(['Max size', spec.additionalConfig.maxSize]);
      if (spec.additionalConfig?.maxObjects) pairs.push(['Max objects', spec.additionalConfig.maxObjects]);

      let connection = '<p class="plugin-text">Connection details appear once the claim is Bound.</p>';
      if (cm) {
        connection = renderDefList([
          ['Endpoint', `http://${cm.data?.BUCKET_HOST ?? '?'}:${cm.data?.BUCKET_PORT ?? '?'}`],
          ['Bucket', cm.data?.BUCKET_NAME ?? '—'],
        ]);
      }

      // Without the client-side uniqueness pre-check, a taken exact name
      // surfaces here: the provisioner leaves the claim Pending and retries
      // with backoff, so say what Pending can mean.
      const phase = claim.status?.phase ?? 'Unknown';
      let pendingNote = '';
      if (phase !== 'Bound') {
        pendingNote = spec.bucketName
          ? `<p class="plugin-hint">The provisioner has not fulfilled this claim yet and retries with backoff. For an exact bucket name this can mean the name is already taken on this store — in or outside Kubernetes. If it stays Pending, delete the claim and pick another name.</p>`
          : `<p class="plugin-hint">The provisioner has not fulfilled this claim yet; it retries with backoff.</p>`;
      }

      // The Secret's name is shown; its values never reach this iframe.
      const mount = `envFrom:
  - configMapRef: { name: ${name} }
  - secretRef: { name: ${name} }`;
      sheetBody.lastElementChild.outerHTML = `
        ${renderDefList(pairs)}
        ${pendingNote}
        ${connection}
        <p class="plugin-hint">
          Credentials are in the Secret <code>${escapeHtml(name)}</code> (same namespace),
          as AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY. Give a pod both with:
        </p>
        <pre class="plugin-text"><code>${escapeHtml(mount)}</code></pre>
        <div class="plugin-actions">
          <button type="button" class="plugin-button-secondary" data-role="close">Close</button>
        </div>
      `;
      sheetBody.querySelector('[data-role="close"]').addEventListener('click', () => closeSheet());
    } catch (err) {
      sheetBody.lastElementChild.outerHTML = errorBox(err);
    }
  }

  async function showCreate() {
    const sheet = await openSheet({ label: 'Create Bucket' });
    if (!sheet) return;
    const { body: sheetBody, close } = sheet;
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

        <div class="plugin-field" data-role="exact-name">
          <label class="plugin-label" for="bucket-name">Exact bucket name</label>
          <input id="bucket-name" name="bucketName" type="text" class="plugin-input" maxlength="63" />
          <span class="plugin-hint">Bucket names are shared across the whole object store. A taken name is only caught by the provisioner: the claim then stays Pending.</span>
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
    // style.display, not the hidden attribute: .plugin-field sets
    // display:flex, which overrides hidden's UA display:none.
    const exactField = sheetBody.querySelector('[data-role="exact-name"]');
    const syncExactField = () => {
      exactField.style.display = modeSelect.value === 'exact' ? '' : 'none';
    };
    syncExactField();
    modeSelect.addEventListener('change', syncExactField);
    // Enter in a <select> submits the form natively; a half-filled bucket
    // form should not submit from the naming dropdown.
    form.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' && e.target.tagName === 'SELECT') e.preventDefault();
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
          // No uniqueness pre-check: it would be client-side and racy. The
          // provisioner is authoritative; a taken name leaves the claim
          // Pending, which the detail sheet explains.
          spec.bucketName = form.querySelector('[name="bucketName"]').value.trim();
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

        const created = await fundament.k8s.create(
          { ...OBC, namespace },
          {
            apiVersion: 'objectbucket.io/v1alpha1',
            kind: 'ObjectBucketClaim',
            metadata: { name, namespace },
            spec,
          },
        );
        close();
        showList();
        await showBucket(name, namespace, created);
      },
    });
  }
}
