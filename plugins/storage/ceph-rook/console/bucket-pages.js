// Buckets section for the ObjectStorage detail page: the ObjectBucketClaims
// provisioned against one store's StorageClass. A claim's detail and the
// create form open in a console sheet (see sheet.js). Claims live
// wherever workloads do, so the list queries cluster-wide; per-project-
// namespace scoping would hide them. Mounted via the OBJECTSTORAGE
// detailSection hook, after the SDK loaded.

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

const OBC = { group: 'objectbucket.io', version: 'v1alpha1', resource: 'objectbucketclaims' };
// The kind the bucket pages are declared under in customComponents.
const OBC_KIND = 'ObjectBucketClaim';
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
    // The namespace's console name where the project gave one: the cluster
    // name (tnt-<project>--<name>) is not what anyone calls it.
    const displayName = (ns) => ctx.namespaceDisplayNames?.[ns] ?? ns;
    body.innerHTML = resourceListHtml(
      claims.map((c) => ({
        name: c.metadata?.name ?? '',
        namespace: c.metadata?.namespace ?? '',
        supportingText: `Namespace ${displayName(c.metadata?.namespace ?? '')} · bucket ${requestedBucket(c.spec)}`,
        badge: phaseBadgeHtml(c.status?.phase),
      })),
      'Buckets',
    ) + failureNote;
    wireListRows(body, (name, namespace) => {
      fundament.openSheet({ kind: OBC_KIND, view: 'detail', resource: { name, namespace } });
    });
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
      ['Namespace', namespace],
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

// The create form, which the Buckets section opens in a console sheet. The
// section hands over the store's StorageClass; the namespaces are the
// project's, the same ones the detail page under the sheet was given.
export function bucketCreatePage(ctx) {
  const sheetBody = document.getElementById('content');
  const storageClassName = ctx.params?.storageClassName;
  const namespaces = ctx.namespaces ?? [];
  if (!storageClassName) {
    sheetBody.innerHTML = errorBox('no object storage given', 'Cannot create a bucket');
    return;
  }
  // A project carries its namespaces; without any (dev sandbox), the
  // namespace is free text instead of a dropdown.
  const namespaceControl = namespaces.length > 0
    ? `<nldd-dropdown><select id="bucket-namespace" name="namespace">
        ${namespaces.map((ns) => `<option value="${escapeHtml(ns)}">${escapeHtml(ns)}</option>`).join('')}
      </select></nldd-dropdown>`
    : `<nldd-text-field id="bucket-namespace" name="namespace" placeholder="default"
                        no-spellcheck></nldd-text-field>`;

  sheetBody.innerHTML = `
    <nldd-rich-text>
      <p>
        The bucket is provisioned on this storage (class
        <code>${escapeHtml(storageClassName)}</code>). The endpoint lands in a
        ConfigMap and the credentials in a Secret, both named after the claim,
        in the claim's namespace.
      </p>
    </nldd-rich-text>
    <nldd-spacer size="12"></nldd-spacer>

    <nldd-form>
      <form novalidate>
        ${errorBannerHtml('bucket-error')}

        ${formFieldHtml('Namespace', namespaceControl, { errorId: 'bucket-namespace-error' })}

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
