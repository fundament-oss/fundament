// Buckets: ObjectBucketClaims against an ObjectStorage's StorageClass. The
// claim row, per-claim view and create form live here, shared by the Buckets
// section on the ObjectStorage detail page (sheets, cluster page) and the
// project-level buckets-* pages.
//
// The section lists cluster-wide: claims live wherever workloads do, so
// per-project-namespace scoping would hide them. Mounted via the
// OBJECTSTORAGE detailSection hook, after the SDK loaded.

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
  formFieldHtml,
  formActionsHtml,
  errorBannerHtml,
} from './_shared.js';

export const OBC = { group: 'objectbucket.io', version: 'v1alpha1', resource: 'objectbucketclaims' };
const CONFIGMAPS = { group: '', version: 'v1', resource: 'configmaps' };

// An ObjectStorage's StorageClass is cephobj-<name> (objectStoreDerivedNamePrefix
// in claims.go); the CRD caps ObjectStorage names at 30.
export const OBJECT_STORAGE_CLASS_PREFIX = 'cephobj-';
export const OBJECT_STORAGE_NAME_MAX = 30;

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

// The bucket StorageClasses this plugin provisions: an ObjectStorage is their
// controller (applyOwnedStorageClass). The cephobj- name prefix is no marker:
// anyone may create a class called that.
export function objectStorageClasses(items) {
  return (items ?? [])
    .flatMap((sc) => {
      const owner = (sc.metadata?.ownerReferences ?? []).find((ref) =>
        ref.controller && ref.kind === 'ObjectStorage' && ref.apiVersion?.startsWith('ceph.fundament.io/'));
      return owner && sc.metadata?.name ? [{ name: sc.metadata.name, store: owner.name }] : [];
    })
    .sort((a, b) => a.store.localeCompare(b.store));
}

// Validates a typed object storage class name; returns an error or null.
export function objectStorageClassError(value) {
  const store = value.startsWith(OBJECT_STORAGE_CLASS_PREFIX)
    ? value.slice(OBJECT_STORAGE_CLASS_PREFIX.length)
    : null;
  if (!store || resourceNameError(store, OBJECT_STORAGE_NAME_MAX)) {
    return `Use ${OBJECT_STORAGE_CLASS_PREFIX}<store name>, where the store name has lowercase letters, digits and dashes, at most ${OBJECT_STORAGE_NAME_MAX} characters.`;
  }
  return null;
}

// classes is objectStorageClasses' result, or null when the list was
// forbidden; only a list that came back empty means there is no store.
export function hasNoObjectStorage(classes) {
  return Array.isArray(classes) && classes.length === 0;
}

// The create form's store picker. Unknown classes take the name as text; a
// single store is preselected, several start on a placeholder.
export function storeFieldHtml(classes) {
  const field = (control, hint) =>
    formFieldHtml('Object storage', control, { errorId: 'bucket-storage-class-error', hint });
  if (classes === null) {
    return field(
      `<nldd-text-field id="bucket-storage-class" name="storageClass" placeholder="${OBJECT_STORAGE_CLASS_PREFIX}my-store"
                        required maxlength="253" no-spellcheck></nldd-text-field>`,
      `The object storage's StorageClass, named ${OBJECT_STORAGE_CLASS_PREFIX}<store name>. The object storages could not be listed; an organization admin sees the store names on the cluster's Object Storage page.`,
    );
  }
  const placeholder = classes.length > 1
    ? '<option value="" selected disabled>Choose an object storage</option>'
    : '';
  const options = classes
    .map((sc) => `<option value="${escapeHtml(sc.name)}"${classes.length === 1 ? ' selected' : ''}>${escapeHtml(sc.store)}</option>`)
    .join('');
  return field(
    `<nldd-dropdown required><select id="bucket-storage-class" name="storageClass">${placeholder}${options}</select></nldd-dropdown>`,
    'The object store the bucket lives on.',
  );
}

// One claim table row. data-name/data-namespace carry the cluster-side
// identity for the row link; displayNames only changes the visible namespace.
export function claimRowHtml(claim, { displayNames = {}, storageClass = false } = {}) {
  const name = claim.metadata?.name ?? '';
  const namespace = claim.metadata?.namespace ?? '';
  const classCell = storageClass ? `<td>${escapeHtml(claim.spec?.storageClassName ?? '—')}</td>` : '';
  return `
        <tr data-name="${escapeHtml(name)}" data-namespace="${escapeHtml(namespace)}">
          <td><a href="#" class="row-link">${escapeHtml(name)}</a></td>
          <td>${escapeHtml(displayNames[namespace] ?? namespace)}</td>
          <td>${escapeHtml(claim.status?.phase ?? 'Unknown')}</td>
          <td>${escapeHtml(requestedBucket(claim.spec))}</td>
          ${classCell}
        </tr>`;
}

// Fetches a claim and its connection ConfigMap, which only appears once the
// claim binds.
export async function loadClaim(namespace, name) {
  const [claim, cm] = await Promise.all([
    fundament.k8s.get({ ...OBC, namespace, name }),
    fundament.k8s.get({ ...CONFIGMAPS, namespace, name }).catch(() => undefined),
  ]);
  return { claim, cm };
}

// The per-claim view: claim state, connection info from the ConfigMap, and a
// pointer to the credentials Secret. The Secret's name is shown; its values
// never reach this iframe.
export function claimDetailHtml(claim, cm, { name, namespace, namespaceLabel = namespace }) {
  const spec = claim.spec ?? {};
  const pairs = [
    ['Phase', claim.status?.phase ?? 'Unknown'],
    ['Namespace', namespaceLabel],
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

  const mount = `envFrom:
  - configMapRef: { name: ${name} }
  - secretRef: { name: ${name} }`;
  return `
        ${renderDefList(pairs)}
        ${pendingNote}
        ${connection}
        <p class="plugin-hint">
          Credentials are in the Secret <code>${escapeHtml(name)}</code> (same namespace),
          as AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY. Give a pod both with:
        </p>
        <pre class="plugin-text"><code>${escapeHtml(mount)}</code></pre>`;
}

// A dropdown of the project's namespaces, labelled with their display names;
// without any (cluster pages), free text.
export function namespaceControlHtml(namespaces, displayNames = {}) {
  return namespaces.length > 0
    ? `<nldd-dropdown><select id="bucket-namespace" name="namespace">
          ${namespaces.map((ns) => `<option value="${escapeHtml(ns)}">${escapeHtml(displayNames[ns] ?? ns)}</option>`).join('')}
        </select></nldd-dropdown>`
    : `<nldd-text-field id="bucket-namespace" name="namespace" placeholder="default"
                          no-spellcheck></nldd-text-field>`;
}

// The create form. storeField is a complete form field for picking the
// StorageClass, or empty when the caller fixes it.
export function bucketFormHtml({ namespaceControl, storeField = '' }) {
  return `
      <nldd-form>
        <form novalidate>
          ${errorBannerHtml('bucket-error')}

          ${formFieldHtml('Namespace', namespaceControl, { errorId: 'bucket-namespace-error' })}

          ${storeField}

          ${formFieldHtml(
            'Name',
            `<nldd-text-field id="claim-name" name="name" placeholder="my-bucket"
                              required maxlength="63" no-spellcheck></nldd-text-field>`,
            {
              errorId: 'claim-name-error',
              hint: 'Names the claim and its ConfigMap/Secret. Lowercase letters, digits and dashes.',
            },
          )}

          ${formFieldHtml(
            'Bucket name',
            `<nldd-dropdown><select id="naming-mode" name="namingMode">
              <option value="generate" selected>Generate from the claim name (recommended)</option>
              <option value="exact">Exact name</option>
            </select></nldd-dropdown>`,
            { hint: 'Generated names get a random suffix, so they cannot collide.' },
          )}

          <div data-role="exact-name">
            ${formFieldHtml(
              'Exact bucket name',
              `<nldd-text-field id="bucket-name" name="bucketName" maxlength="63"
                                no-spellcheck></nldd-text-field>`,
              {
                errorId: 'bucket-name-error',
                hint: 'Bucket names are shared across the whole object store. A taken name is only caught by the provisioner: the claim then stays Pending.',
              },
            )}
          </div>

          ${formFieldHtml(
            'Max size (optional)',
            `<nldd-text-field id="max-size" name="maxSize" placeholder="10Gi"
                              no-spellcheck></nldd-text-field>`,
            { errorId: 'max-size-error', hint: 'Quota for the bucket, e.g. 10Gi. Empty means unlimited.' },
          )}

          ${formFieldHtml(
            'Max objects (optional)',
            `<nldd-text-field id="max-objects" name="maxObjects" no-spellcheck></nldd-text-field>`,
            { errorId: 'max-objects-error', hint: 'Empty means unlimited.' },
          )}

          ${formActionsHtml({ submitId: 'bucket-submit', submitText: 'Create bucket', cancelId: 'bucket-cancel' })}
        </form>
      </nldd-form>`;
}

// Wires the form bucketFormHtml rendered into root. storageClassName reads the
// class at submit time; storeChecks validate the caller's store field.
// onCreated gets the created claim once the API accepted it.
export function wireBucketForm(root, { storageClassName, storeChecks = [], onCancel, onCreated }) {
  const form = root.querySelector('form');
  const nameInput = form.querySelector('[name="name"]');
  const modeSelect = form.querySelector('[name="namingMode"]');
  // style.display, not the hidden attribute: a component's own display
  // rule overrides hidden's UA display:none.
  const exactField = root.querySelector('[data-role="exact-name"]');
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
  root.querySelector('#bucket-cancel').addEventListener('click', () => onCancel());

  wireSubmit(form, {
    button: root.querySelector('#bucket-submit'),
    errorBanner: root.querySelector('#bucket-error'),
    failPrefix: 'Failed to create bucket',
    checks: [
      [form.querySelector('[name="namespace"]'), (value) => (value ? null : 'Please choose a namespace.')],
      ...storeChecks,
      [nameInput, (value) => resourceNameError(value, 63)],
      [
        form.querySelector('[name="bucketName"]'),
        (value) => (modeSelect.value === 'exact' ? bucketNameError(value) : null),
      ],
      [form.querySelector('[name="maxSize"]'), (value) => (value ? quantityError(value) : null)],
      [
        form.querySelector('[name="maxObjects"]'),
        (value) =>
          value && (!/^[0-9]+$/.test(value) || Number(value) < 1)
            ? 'Max objects must be a whole number of at least 1.'
            : null,
      ],
    ],
    action: async () => {
      const name = nameInput.value.trim();
      const namespace = form.querySelector('[name="namespace"]').value.trim();
      const spec = { storageClassName: storageClassName() };

      if (modeSelect.value === 'exact') {
        // No uniqueness pre-check: it would be client-side and racy. The
        // provisioner is authoritative; a taken name leaves the claim
        // Pending, which the detail view explains.
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
      await onCreated(created, name, namespace);
    },
  });
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
      // Project members may only list in their namespaces; on cluster pages
      // there are none, so one cluster-wide list is used.
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
    tbody.innerHTML = claims.map((c) => claimRowHtml(c)).join('') + failureNote;
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
      const { claim, cm } = preloaded ? { claim: preloaded } : await loadClaim(namespace, name);
      sheetBody.lastElementChild.outerHTML = `
        ${claimDetailHtml(claim, cm, { name, namespace })}
        <nldd-spacer size="16"></nldd-spacer>
        <nldd-button type="button" variant="secondary" text="Close" data-role="close"></nldd-button>
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

    sheetBody.insertAdjacentHTML('beforeend', `
      <nldd-rich-text>
        <p>
          The bucket is provisioned on this storage (class
          <code>${escapeHtml(storageClassName)}</code>). The endpoint lands in a
          ConfigMap and the credentials in a Secret, both named after the claim,
          in the claim's namespace.
        </p>
      </nldd-rich-text>
      <nldd-spacer size="12"></nldd-spacer>
      ${bucketFormHtml({ namespaceControl: namespaceControlHtml(namespaces) })}
    `);

    wireBucketForm(sheetBody, {
      storageClassName: () => storageClassName,
      onCancel: close,
      onCreated: async (created, name, namespace) => {
        close();
        showList();
        await showBucket(name, namespace, created);
      },
    });
  }
}
