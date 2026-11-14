// Buckets: ObjectBucketClaims against an ObjectStorage's StorageClass. The
// Buckets section on the ObjectStorage detail page and the project's Buckets
// page (buckets-list.js) both open a claim's detail and the create form in a
// console sheet (see sheet.js).
//
// The section lists cluster-wide: claims live wherever workloads do, so
// per-project-namespace scoping would hide them. Mounted via the
// OBJECTSTORAGE detailSection hook, after the SDK loaded.

import {
  emptyStateHtml,
  errorBox,
  hintHtml,
  latestRead,
  loadingHtml,
  phaseBadgeHtml,
  quantityError,
  escapeHtml,
  renderKeyValueList,
  resourceListHtml,
  resourceNameError,
  sectionHtml,
  wireListRows,
  wireSubmit,
  formFieldHtml,
  formActionsHtml,
  errorBannerHtml,
} from './_shared.js';

export const OBC = { group: 'objectbucket.io', version: 'v1alpha1', resource: 'objectbucketclaims' };
// The kind the bucket pages are declared under in customComponents.
export const OBC_KIND = 'ObjectBucketClaim';
const CONFIGMAPS = { group: '', version: 'v1', resource: 'configmaps' };
const STORAGECLASSES = { group: 'storage.k8s.io', version: 'v1', resource: 'storageclasses' };

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

// The claims as a list. The rows carry the cluster-side namespace for the
// link; displayNames only changes the namespace shown, since the cluster name
// (tnt-<project>--<name>) is not what anyone calls it.
export function claimListHtml(claims, { displayNames = {}, storageClass = false } = {}) {
  return resourceListHtml(
    claims.map((c) => {
      const namespace = c.metadata?.namespace ?? '';
      const facts = [`Namespace ${displayNames[namespace] ?? namespace}`, `bucket ${requestedBucket(c.spec)}`];
      if (storageClass) facts.push(`class ${c.spec?.storageClassName ?? '—'}`);
      return {
        name: c.metadata?.name ?? '',
        namespace,
        supportingText: facts.join(' · '),
        badge: phaseBadgeHtml(c.status?.phase),
      };
    }),
    'Buckets',
  );
}

// Opens a claim's detail in a sheet when one of the rows is clicked.
export function wireClaimLinks(root) {
  wireListRows(root, (name, namespace) => {
    fundament.openSheet({ kind: OBC_KIND, view: 'detail', resource: { name, namespace } });
  });
}

export function mountBucketsSection(container, item, ctx, cfg) {
  const storageClassName = item.status?.storageClassName ?? `${cfg.storageClassPrefix}${item.metadata?.name}`;
  const namespaces = ctx.namespaces ?? [];

  container.innerHTML = `
    <nldd-spacer size="24"></nldd-spacer>
    ${sectionHtml(
      'Buckets',
      `<nldd-button data-role="create" type="button" appearance="primary" start-icon="plus-small"
                    text="New bucket…"></nldd-button>
      <nldd-spacer size="16"></nldd-spacer>
      <div class="buckets-body"></div>`,
    )}
  `;
  // The create page needs the class its claim is provisioned against.
  container.querySelector('[data-role="create"]').addEventListener('click', () => {
    fundament.openSheet({ kind: OBC_KIND, view: 'create', params: { storageClassName } });
  });
  const body = container.querySelector('.buckets-body');

  // A bucket created, or a sheet replaced: the list may be out of date.
  const nextRead = latestRead();
  fundament.onResourcesChanged(() => showList());
  showList();

  async function showList() {
    const isCurrent = nextRead();
    body.innerHTML = loadingHtml('Loading buckets…');

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
      if (isCurrent()) body.innerHTML = errorBox(err);
      return;
    }
    if (!isCurrent()) return;

    const failureNote = failedNamespaces.length === 0
      ? ''
      : `<nldd-spacer size="12"></nldd-spacer>${errorBox(
        `listing failed in ${failedNamespaces.join(', ')}`,
        'Some buckets may be missing',
      )}`;
    if (claims.length === 0) {
      body.innerHTML = failureNote || emptyStateHtml({
        icon: 'cylinder-split-slash',
        text: 'No buckets on this storage yet',
      });
      return;
    }
    body.innerHTML = claimListHtml(claims, { displayNames: ctx.namespaceDisplayNames }) + failureNote;
    wireClaimLinks(body);
  }
}

// One claim's detail, which the Buckets section opens in a console sheet,
// and the create form opens in its own place once the claim exists.
export async function bucketDetailPage(ctx) {
  const sheetBody = document.getElementById('content');
  const name = ctx.resource?.name;
  const namespace = ctx.resource?.namespace;
  if (!name || !namespace) {
    sheetBody.innerHTML = errorBox('no bucket claim given', 'Cannot show the bucket');
    return;
  }
  sheetBody.innerHTML = loadingHtml();
  try {
    const [claim, cm] = await Promise.all([
      fundament.k8s.get({ ...OBC, namespace, name }),
      // The connection ConfigMap only appears once the claim binds.
      fundament.k8s.get({ ...CONFIGMAPS, namespace, name }).catch(() => undefined),
    ]);
    const spec = claim.spec ?? {};
    const pairs = [
      ['Phase', claim.status?.phase ?? 'Unknown'],
      ['Namespace', ctx.namespaceDisplayNames?.[namespace] ?? namespace],
      [spec.bucketName ? 'Requested bucket name' : 'Bucket name prefix',
        spec.bucketName ?? spec.generateBucketName ?? '—'],
    ];
    if (spec.additionalConfig?.maxSize) pairs.push(['Max size', spec.additionalConfig.maxSize]);
    if (spec.additionalConfig?.maxObjects) pairs.push(['Max objects', spec.additionalConfig.maxObjects]);

    let connection = hintHtml('Connection details appear once the claim is Bound.');
    if (cm) {
      connection = renderKeyValueList([
        ['Endpoint', `http://${cm.data?.BUCKET_HOST ?? '?'}:${cm.data?.BUCKET_PORT ?? '?'}`],
        ['Bucket', cm.data?.BUCKET_NAME ?? '—'],
      ], 'Connection');
    }

    // Without the client-side uniqueness pre-check, a taken exact name
    // surfaces here: the provisioner leaves the claim Pending and retries
    // with backoff, so say what Pending can mean.
    const phase = claim.status?.phase ?? 'Unknown';
    let pendingNote = '';
    if (phase !== 'Bound') {
      pendingNote = spec.bucketName
        ? `<nldd-spacer size="12"></nldd-spacer>${hintHtml('The provisioner has not fulfilled this claim yet and retries with backoff. For an exact bucket name this can mean the name is already taken on this store, in or outside Kubernetes. If it stays Pending, delete the claim and pick another name.')}`
        : `<nldd-spacer size="12"></nldd-spacer>${hintHtml('The provisioner has not fulfilled this claim yet; it retries with backoff.')}`;
    }

    // The Secret's name is shown; its values never reach this iframe.
    const mount = `envFrom:
  - configMapRef: { name: ${name} }
  - secretRef: { name: ${name} }`;
    sheetBody.innerHTML = `
      ${renderKeyValueList(pairs, 'Bucket claim')}
      ${pendingNote}
      <nldd-spacer size="24"></nldd-spacer>
      <nldd-title size="5"><h2>Connection</h2></nldd-title>
      <nldd-spacer size="12"></nldd-spacer>
      ${connection}
      <nldd-spacer size="16"></nldd-spacer>
      <nldd-rich-text>
        <p>
          Credentials are in the Secret <code>${escapeHtml(name)}</code> (same namespace),
          as AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY. Give a pod both with:
        </p>
        <pre><code>${escapeHtml(mount)}</code></pre>
      </nldd-rich-text>
      <nldd-spacer size="24"></nldd-spacer>
    `;
    // No Close of its own: the console's sheet has one.
  } catch (err) {
    sheetBody.innerHTML = errorBox(err);
  }
}

// Project members may list StorageClasses. Should the list still be
// forbidden, null means unknown and the form takes the class by name.
async function loadObjectStorageClasses() {
  try {
    const { items } = await fundament.k8s.list(STORAGECLASSES);
    return objectStorageClasses(items);
  } catch (err) {
    if (err?.code === 'forbidden') return null;
    throw err;
  }
}

// The create form, in a console sheet. The Buckets section hands over its
// store's StorageClass; from the project's Buckets page the user picks the
// store. The namespaces are the project's.
export async function bucketCreatePage(ctx) {
  const sheetBody = document.getElementById('content');
  const fixedClass = ctx.params?.storageClassName;
  const namespaces = ctx.namespaces ?? [];
  const displayNames = ctx.namespaceDisplayNames ?? {};

  let storeField = '';
  let intro = `The bucket is provisioned on this storage (class
        <code>${escapeHtml(fixedClass ?? '')}</code>).`;
  if (!fixedClass) {
    sheetBody.innerHTML = loadingHtml();
    let classes;
    try {
      classes = await loadObjectStorageClasses();
    } catch (err) {
      sheetBody.innerHTML = errorBox(err, 'Cannot create a bucket');
      return;
    }
    if (hasNoObjectStorage(classes)) {
      sheetBody.innerHTML = emptyStateHtml({
        icon: 'cylinder-split-slash',
        text: 'No object storage on this cluster',
        supportingText: 'An organization admin creates it on the cluster page.',
      });
      return;
    }
    storeField = storeFieldHtml(classes);
    intro = 'The bucket is provisioned on the object storage you pick.';
  }

  // A project carries its namespaces; without any (dev sandbox), the
  // namespace is free text instead of a dropdown.
  const namespaceControl = namespaces.length > 0
    ? `<nldd-dropdown><select id="bucket-namespace" name="namespace">
        ${namespaces.map((ns) => `<option value="${escapeHtml(ns)}">${escapeHtml(displayNames[ns] ?? ns)}</option>`).join('')}
      </select></nldd-dropdown>`
    : `<nldd-text-field id="bucket-namespace" name="namespace" placeholder="default"
                        no-spellcheck></nldd-text-field>`;

  sheetBody.innerHTML = `
    <nldd-rich-text>
      <p>
        ${intro} The endpoint lands in a ConfigMap and the credentials in a
        Secret, both named after the claim, in the claim's namespace.
      </p>
    </nldd-rich-text>
    <nldd-spacer size="12"></nldd-spacer>

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
    </nldd-form>
  `;

  const form = sheetBody.querySelector('form');
  const nameInput = form.querySelector('[name="name"]');
  const storeInput = form.querySelector('[name="storageClass"]');
  const storeChecks = !storeInput ? [] : [[
    storeInput,
    storeInput.tagName === 'SELECT'
      ? (value) => (value ? null : 'Please choose an object storage.')
      : objectStorageClassError,
  ]];
  const modeSelect = form.querySelector('[name="namingMode"]');
  // style.display, not the hidden attribute: a component's own display
  // rule overrides hidden's UA display:none.
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
  sheetBody.querySelector('#bucket-cancel').addEventListener('click', () => {
    fundament.closeSheet();
  });

  wireSubmit(form, {
    button: sheetBody.querySelector('#bucket-submit'),
    errorBanner: sheetBody.querySelector('#bucket-error'),
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
      const spec = { storageClassName: fixedClass ?? storeInput.value.trim() };

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

      await fundament.k8s.create(
        { ...OBC, namespace },
        {
          apiVersion: 'objectbucket.io/v1alpha1',
          kind: 'ObjectBucketClaim',
          metadata: { name, namespace },
          spec,
        },
      );
      // The claim's own sheet takes this one's place, which also tells the
      // Buckets section under it to list again.
      fundament.openSheet({ kind: OBC_KIND, view: 'detail', resource: { name, namespace } });
    },
  });
}
