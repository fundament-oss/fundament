// Page factory for the consumer kinds (BlockStorage, FileStorage,
// ObjectStorage): one implementation of the list and detail flows,
// parameterized per kind; create and edit open in an <nldd-sheet> from those
// pages. The kinds differ only in labels/texts and a few kind-specific spec
// fields, so each page file reduces to a factory call with its kind's config.

import {
  loadSdk,
  openSheet,
  errorBox,
  ensureNldd,
  fetchCephCapacity,
  humanizeQuantity,
  escapeHtml,
  emptyRow,
  errorRow,
  wireRowLinks,
  navigateToDetail,
  renderDefList,
  replicasFieldHtml,
  replicasValue,
  defaultFieldHtml,
  integerFieldHtml,
  integerFieldError,
  resourceNameError,
  wireSubmit,
  formFieldHtml,
  formActionsHtml,
  errorBannerHtml,
} from './_shared.js';
import { mountBucketsSection } from './bucket-pages.js';

const GROUP_VERSION = { group: 'ceph.fundament.io', version: 'v1alpha1' };

// Descriptors for the kind-specific integer spec fields. Everywhere a field
// surfaces — forms, validation, spec building, list cells, detail pairs —
// iterates cfg.intFields, so a field cannot be rendered in one form and
// forgotten in another.
const METADATA_SERVERS = {
  name: 'metadataServers',
  id: 'mds-count',
  label: 'Metadata servers',
  column: 'MDS',
  hint: 'Active MDS daemons; each gets a standby. 1 is right unless metadata throughput at scale demands more.',
  min: 1,
  max: 5,
  default: 1,
};

const GATEWAY_INSTANCES = {
  name: 'gatewayInstances',
  id: 'rgw-count',
  label: 'Gateway instances',
  column: 'Gateways',
  hint: 'Gateway pods serving the S3 API. 1 is right unless request throughput demands more.',
  min: 1,
  max: 5,
  default: 1,
};

export const BLOCKSTORAGE = {
  resource: 'blockstorages',
  kind: 'BlockStorage',
  label: 'Block Storage',
  emptyMessage: 'No block storage.',
  noneSelected: 'No block storage selected.',
  storageClassPrefix: 'ceph-',
  detailHint: "Volumes are placed across all of the shared Ceph cluster's disks.",
  createIntro: `Block storage provides ReadWriteOnce volumes, each mounted by one node at a
    time, over the shared Ceph cluster's disks. It needs at least one DiskPool
    contributing disks; without one it stays Degraded.`,
  intFields: [],
  defaultToggle: true,
  nameMaxLength: 63,
};

export const FILESTORAGE = {
  resource: 'filestorages',
  kind: 'FileStorage',
  label: 'File Storage',
  emptyMessage: 'No file storage.',
  noneSelected: 'No file storage selected.',
  storageClassPrefix: 'cephfs-',
  detailHint: 'Volumes can be mounted by many pods across nodes.',
  createIntro: `File storage provides ReadWriteMany volumes over the shared Ceph cluster's
    disks, which many pods on many nodes can mount at once. It needs at least one
    DiskPool contributing disks; without one it stays Degraded.`,
  intFields: [METADATA_SERVERS],
  defaultToggle: false,
  // The CRD caps FileStorage names at 56: Rook derives a cephfs-<name> label
  // capped at 63. Enforced here too so the form rejects it before the server.
  nameMaxLength: 56,
};

export const OBJECTSTORAGE = {
  resource: 'objectstorages',
  kind: 'ObjectStorage',
  label: 'Object Storage',
  emptyMessage: 'No object storage.',
  noneSelected: 'No object storage selected.',
  storageClassPrefix: 'cephobj-',
  detailHint: `Create buckets below; each bucket's endpoint and credentials are
    stored in its namespace, named after the bucket.`,
  createIntro: `Object storage provides S3-compatible buckets over the shared Ceph cluster's
    disks. It needs at least one DiskPool contributing disks; without one it
    stays Degraded.`,
  intFields: [GATEWAY_INSTANCES],
  defaultToggle: false,
  // The CRD caps ObjectStorage names at 30: Rook rejects CephObjectStore
  // names over 38 and the derived name is cephobj-<name>. Enforced here too
  // so the form rejects it first.
  nameMaxLength: 30,
  // The detail page embeds the Buckets section: the ObjectBucketClaims
  // provisioned against this store's StorageClass.
  detailSection: mountBucketsSection,
};

function intFieldValue(form, field) {
  return Number(form.querySelector(`[name="${field.name}"]`).value);
}

// intFieldChecks validates the kind's integer fields, each under its own field.
function intFieldChecks(cfg, form) {
  return cfg.intFields.map((field) => [
    form.querySelector(`[name="${field.name}"]`),
    (value) => integerFieldError(field, Number(value)),
  ]);
}

// intFieldsHtml renders the kind's integer inputs for a create or edit form;
// spec is undefined on create, so every field falls back to its default.
function intFieldsHtml(cfg, spec) {
  return cfg.intFields
    .map((field) => integerFieldHtml(field, spec?.[field.name] ?? field.default))
    .join('');
}

// specFrom reads the create/edit form into a spec object.
function specFrom(cfg, form) {
  const spec = { replicas: replicasValue(form) };
  for (const field of cfg.intFields) spec[field.name] = intFieldValue(form, field);
  // Always sent, so unticking the box merge-patches the field back to false.
  if (cfg.defaultToggle) spec.default = form.querySelector('[name="default"]').checked;
  return spec;
}

export async function consumerListPage(cfg) {
  // The header row comes from the same cfg.intFields the body cells do, so
  // the two cannot drift; the HTML ships an empty table shell. Rendered
  // before the SDK loads so the table never shows headerless.
  const headers = [
    'Name', 'Phase', 'Storage Class', 'Replicas',
    ...cfg.intFields.map((field) => field.column),
    'Failure Domain', 'Message',
  ];
  // Null-guarded: a stale cached pre-0.3.0 page has a static <thead> without
  // id="head", and throwing here would kill the whole module.
  const head = document.getElementById('head');
  if (head) {
    head.innerHTML = `<tr>${headers.map((h) => `<th>${escapeHtml(h)}</th>`).join('')}</tr>`;
  }
  const colspan = headers.length;
  const tbody = document.getElementById('rows');
  tbody.innerHTML = emptyRow(colspan, 'Loading…');

  await loadSdk();
  await fundament.init;
  // Warm the sheet bundle so the first Create click opens instantly.
  ensureNldd().catch(() => {});

  document.getElementById('create-btn').addEventListener('click', async () => {
    const sheet = await openSheet({ label: `Create ${cfg.label}` });
    if (sheet) renderCreateForm(cfg, sheet.body, sheet.close);
  });

  try {
    const { items } = await fundament.k8s.list({ ...GROUP_VERSION, resource: cfg.resource });

    if (!items || items.length === 0) {
      tbody.innerHTML = emptyRow(colspan, cfg.emptyMessage);
      return;
    }
    tbody.innerHTML = items
      .map((item) => {
        const status = item.status ?? {};
        const name = item.metadata?.name ?? '';
        const intFieldCells = cfg.intFields
          .map((field) => `<td>${escapeHtml(String(item.spec?.[field.name] ?? field.default))}</td>`)
          .join('');
        return `
          <tr data-name="${escapeHtml(name)}">
            <td><a href="#" class="row-link">${escapeHtml(name)}</a></td>
            <td>${escapeHtml(status.phase ?? 'Unknown')}</td>
            <td>${escapeHtml(status.storageClassName ?? '—')}</td>
            <td>${escapeHtml(String(status.replicas ?? '—'))}</td>
            ${intFieldCells}
            <td>${escapeHtml(status.failureDomain ?? '—')}</td>
            <td>${escapeHtml(status.message ?? '')}</td>
          </tr>`;
      })
      .join('');
    wireRowLinks(tbody);
  } catch (err) {
    tbody.innerHTML = errorRow(colspan, err);
  }
}

export async function consumerDetailPage(cfg) {
  await loadSdk();
  const ctx = await fundament.init;
  ensureNldd().catch(() => {});

  const content = document.getElementById('content');
  const heading = document.getElementById('heading');
  const actions = document.getElementById('actions');
  const resource = { ...GROUP_VERSION, resource: cfg.resource };
  const name = ctx.resource?.name;

  function renderReadOnly(item) {
    const status = item.status ?? {};

    const pairs = [
      ['Phase', status.phase ?? 'Unknown'],
      ['Storage Class', status.storageClassName ?? '—'],
      ['Replicas', String(status.replicas ?? '—')],
    ];
    for (const field of cfg.intFields) {
      pairs.push([field.label, String(item.spec?.[field.name] ?? field.default)]);
    }
    if (cfg.defaultToggle) pairs.push(['Default StorageClass', item.spec?.default ? 'Yes' : 'No']);
    pairs.push(['Failure Domain', status.failureDomain ?? '—']);
    if (status.message) pairs.push(['Message', status.message]);

    return `
      <h2 class="plugin-heading">Status</h2>
      ${renderDefList(pairs)}
      <p class="plugin-hint">${cfg.detailHint}</p>
      <p class="plugin-hint" data-role="capacity"></p>
    `;
  }

  async function showDetail() {
    try {
      const item = await fundament.k8s.get({ ...resource, name });
      heading.textContent = `${cfg.label} · ${item.metadata?.name ?? name}`;
      content.innerHTML = renderReadOnly(item);
      actions.hidden = false;
      // .onclick, not addEventListener: this button lives outside #content and
      // survives every re-render, so listeners would stack. (CSP restricts inline
      // handler *attributes*, not this.)
      document.getElementById('edit-btn').onclick = () => showEdit(item);
      // Filled in after the fact: users cannot run ceph df, but the
      // CephCluster status carries the raw numbers.
      fetchCephCapacity().then((cap) => {
        const el = content.querySelector('[data-role="capacity"]');
        if (!cap || !el) return;
        el.textContent = `Raw free space: ${humanizeQuantity(String(cap.bytesAvailable))} of `
          + `${humanizeQuantity(String(cap.bytesTotal))} across the shared cluster; `
          + 'divide by the replica count for usable space.';
      });
      return item;
    } catch (err) {
      actions.hidden = true;
      content.innerHTML = errorBox(err);
      return undefined;
    }
  }

  async function showEdit(item) {
    const sheet = await openSheet({ label: `Edit ${cfg.label}` });
    if (!sheet) return;
    const { body, close } = sheet;
    body.insertAdjacentHTML('beforeend', `
      <nldd-form>
        <form novalidate>
          ${errorBannerHtml('edit-error')}

          ${replicasFieldHtml(item.spec?.replicas)}

          ${intFieldsHtml(cfg, item.spec)}

          ${cfg.defaultToggle ? defaultFieldHtml(item.spec?.default === true) : ''}

          ${formActionsHtml({ submitId: 'save-btn', submitText: 'Save', cancelId: 'cancel-btn' })}
        </form>
      </nldd-form>
    `);

    const form = body.querySelector('form');
    body.querySelector('#cancel-btn').addEventListener('click', () => close());

    wireSubmit(form, {
      button: body.querySelector('#save-btn'),
      errorBanner: body.querySelector('#edit-error'),
      failPrefix: 'Failed to save',
      checks: intFieldChecks(cfg, form),
      action: async () => {
        // Merge-patch of spec only: status is untouched.
        await fundament.k8s.patch({ ...resource, name }, { spec: specFrom(cfg, form) });
        close();
        await showDetail();
      },
    });
  }

  if (!name) {
    content.textContent = cfg.noneSelected;
    return;
  }
  const item = await showDetail();
  // The kind-specific extra section (e.g. ObjectStorage's Buckets) lives in
  // #extra, outside #content, so the edit form's re-renders never touch it.
  const extra = document.getElementById('extra');
  if (item && extra && cfg.detailSection) cfg.detailSection(extra, item, ctx, cfg);
}

// renderCreateForm fills a sheet with the kind's create form; the list
// page opens it.
function renderCreateForm(cfg, body, close) {
  body.insertAdjacentHTML('beforeend', `
    <nldd-rich-text>
      <p>${cfg.createIntro}</p>
    </nldd-rich-text>
    <nldd-spacer size="12"></nldd-spacer>

    <nldd-form>
      <form novalidate>
        ${errorBannerHtml('error-banner')}

        ${formFieldHtml(
          'Name',
          `<nldd-text-field id="consumer-name" name="name" placeholder="default"
                            required maxlength="${cfg.nameMaxLength}" no-spellcheck></nldd-text-field>`,
          {
            errorId: 'consumer-name-error',
            hint: `Lowercase letters, digits and dashes. Names the resulting StorageClass (prefixed ${cfg.storageClassPrefix}).`,
          },
        )}

        ${replicasFieldHtml()}

        ${intFieldsHtml(cfg)}

        ${cfg.defaultToggle ? defaultFieldHtml() : ''}

        ${formActionsHtml({ submitId: 'submit-btn', submitText: `Create ${cfg.label}`, cancelId: 'cancel-btn' })}
      </form>
    </nldd-form>
  `);

  const form = body.querySelector('form');
  const nameInput = form.querySelector('[name="name"]');

  body.querySelector('#cancel-btn').addEventListener('click', () => close());

  wireSubmit(form, {
    button: body.querySelector('#submit-btn'),
    errorBanner: body.querySelector('#error-banner'),
    failPrefix: `Failed to create ${cfg.label}`,
    checks: [
      [nameInput, (value) => resourceNameError(value, cfg.nameMaxLength)],
      ...intFieldChecks(cfg, form),
    ],
    action: async () => {
      const name = nameInput.value.trim();
      await fundament.k8s.create(
        { ...GROUP_VERSION, resource: cfg.resource },
        {
          apiVersion: 'ceph.fundament.io/v1alpha1',
          kind: cfg.kind,
          metadata: { name },
          spec: specFrom(cfg, form),
        },
      );
      close();
      navigateToDetail(name);
    },
  });
}
