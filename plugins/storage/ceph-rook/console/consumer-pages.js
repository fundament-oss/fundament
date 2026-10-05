// Page factory for the consumer kinds (BlockStorage, FileStorage,
// ObjectStorage): one implementation of the list/detail/create flows,
// parameterized per kind. The kinds differ only in labels/texts and a few
// kind-specific spec fields, so each page file reduces to a factory call with
// its kind's config.

import {
  loadSdk,
  escapeHtml,
  emptyRow,
  errorRow,
  wireRowLinks,
  navigateToCreate,
  navigateToDetail,
  navigateBack,
  renderDefList,
  replicasFieldHtml,
  replicasValue,
  defaultFieldHtml,
  metadataServersFieldHtml,
  metadataServersError,
  gatewayInstancesFieldHtml,
  gatewayInstancesError,
  resourceNameError,
  wireSubmit,
} from './_shared.js';

const GROUP_VERSION = { group: 'ceph.fundament.io', version: 'v1alpha1' };

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
  metadataServers: false,
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
  metadataServers: true,
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
  detailHint: `Create an ObjectBucketClaim referencing this StorageClass to provision a
    bucket; its S3 endpoint and credentials land in a ConfigMap and Secret named
    after the claim, and spec.additionalConfig (maxSize, maxObjects) caps a
    claim's bucket.`,
  createIntro: `Object storage provides S3-compatible buckets over the shared Ceph cluster's
    disks, served by RGW gateway pods. It needs at least one DiskPool
    contributing disks; without one it stays Degraded.`,
  metadataServers: false,
  defaultToggle: false,
  gatewayInstances: true,
  // The CRD caps ObjectStorage names at 55: Rook derives a cephobj-<name>
  // label capped at 63. Enforced here too so the form rejects it first.
  nameMaxLength: 55,
};

function metadataServersValue(form) {
  return Number(form.querySelector('[name="metadataServers"]').value);
}

function gatewayInstancesValue(form) {
  return Number(form.querySelector('[name="gatewayInstances"]').value);
}

// fieldsError validates the kind-specific numeric fields; first error wins.
function fieldsError(cfg, form) {
  if (cfg.metadataServers) {
    const invalid = metadataServersError(metadataServersValue(form));
    if (invalid) return invalid;
  }
  if (cfg.gatewayInstances) {
    const invalid = gatewayInstancesError(gatewayInstancesValue(form));
    if (invalid) return invalid;
  }
  return null;
}

// specFrom reads the create/edit form into a spec object.
function specFrom(cfg, form) {
  const spec = { replicas: replicasValue(form) };
  if (cfg.metadataServers) spec.metadataServers = metadataServersValue(form);
  if (cfg.gatewayInstances) spec.gatewayInstances = gatewayInstancesValue(form);
  // Always sent, so unticking the box merge-patches the field back to false.
  if (cfg.defaultToggle) spec.default = form.querySelector('[name="default"]').checked;
  return spec;
}

export async function consumerListPage(cfg) {
  await loadSdk();
  await fundament.init;

  const tbody = document.getElementById('rows');
  const colspan = 6 + (cfg.metadataServers ? 1 : 0) + (cfg.gatewayInstances ? 1 : 0);

  document.getElementById('create-btn').addEventListener('click', () => navigateToCreate());

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
        const metadataServersCell = cfg.metadataServers
          ? `<td>${escapeHtml(String(item.spec?.metadataServers ?? 1))}</td>`
          : '';
        const gatewayInstancesCell = cfg.gatewayInstances
          ? `<td>${escapeHtml(String(item.spec?.gatewayInstances ?? 1))}</td>`
          : '';
        return `
          <tr data-name="${escapeHtml(name)}">
            <td><a href="#" class="row-link">${escapeHtml(name)}</a></td>
            <td>${escapeHtml(status.phase ?? 'Unknown')}</td>
            <td>${escapeHtml(status.storageClassName ?? '—')}</td>
            <td>${escapeHtml(String(status.replicas ?? '—'))}</td>
            ${metadataServersCell}
            ${gatewayInstancesCell}
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
    if (cfg.metadataServers) pairs.push(['Metadata servers', String(item.spec?.metadataServers ?? 1)]);
    if (cfg.gatewayInstances) pairs.push(['Gateway instances', String(item.spec?.gatewayInstances ?? 1)]);
    if (cfg.defaultToggle) pairs.push(['Default StorageClass', item.spec?.default ? 'Yes' : 'No']);
    pairs.push(['Failure Domain', status.failureDomain ?? '—']);
    if (status.message) pairs.push(['Message', status.message]);

    return `
      <h2 class="plugin-heading">Status</h2>
      ${renderDefList(pairs)}
      <p class="plugin-hint">
        ${cfg.detailHint} <code>ceph df</code> shows free space.
      </p>
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
    } catch (err) {
      actions.hidden = true;
      content.innerHTML = `<div class="plugin-error">${escapeHtml(
        `Failed to load: ${err?.message ?? err}`,
      )}</div>`;
    }
  }

  async function showEdit(item) {
    actions.hidden = true;

    content.innerHTML = `
      <form id="edit-form" class="plugin-form" novalidate>
        <div class="plugin-error" id="edit-error" hidden></div>

        ${replicasFieldHtml(item.spec?.replicas)}

        ${cfg.metadataServers ? metadataServersFieldHtml(item.spec?.metadataServers ?? 1) : ''}

        ${cfg.gatewayInstances ? gatewayInstancesFieldHtml(item.spec?.gatewayInstances ?? 1) : ''}

        ${cfg.defaultToggle ? defaultFieldHtml(item.spec?.default === true) : ''}

        <div class="plugin-actions">
          <button type="submit" class="plugin-button" id="save-btn">Save</button>
          <button type="button" class="plugin-button-secondary" id="cancel-btn">Cancel</button>
        </div>
      </form>
    `;

    const form = document.getElementById('edit-form');

    document.getElementById('cancel-btn').addEventListener('click', () => showDetail());

    wireSubmit(form, {
      button: document.getElementById('save-btn'),
      errorBox: document.getElementById('edit-error'),
      busyLabel: 'Saving…',
      failPrefix: 'Failed to save',
      validate: () => fieldsError(cfg, form),
      action: async () => {
        // Merge-patch of spec only: status is untouched.
        await fundament.k8s.patch({ ...resource, name }, { spec: specFrom(cfg, form, true) });
        await showDetail();
      },
    });
  }

  if (!name) {
    content.textContent = cfg.noneSelected;
  } else {
    await showDetail();
  }
}

export async function consumerCreatePage(cfg) {
  await loadSdk();
  await fundament.init;

  const content = document.getElementById('content');

  content.innerHTML = `
    <p class="plugin-text">
      ${cfg.createIntro}
    </p>

    <form id="create-form" class="plugin-form" novalidate>
      <div class="plugin-error" id="error-box" hidden></div>

      <div class="plugin-field">
        <label class="plugin-label" for="consumer-name">Name</label>
        <input id="consumer-name" name="name" type="text" class="plugin-input"
               placeholder="default" required
               pattern="[a-z0-9]([a-z0-9\\-]*[a-z0-9])?" maxlength="${cfg.nameMaxLength}" />
        <span class="plugin-hint">Lowercase letters, digits and dashes. Names the resulting StorageClass (prefixed ${cfg.storageClassPrefix}).</span>
      </div>

      ${replicasFieldHtml()}

      ${cfg.metadataServers ? metadataServersFieldHtml() : ''}

      ${cfg.defaultToggle ? defaultFieldHtml() : ''}

      <div class="plugin-actions">
        <button id="submit-btn" type="submit" class="plugin-button">Create ${cfg.kind}</button>
        <button id="cancel-btn" type="button" class="plugin-button-secondary">Cancel</button>
      </div>
    </form>
  `;

  const form = document.getElementById('create-form');
  const nameInput = form.querySelector('[name="name"]');

  document.getElementById('cancel-btn').addEventListener('click', () => navigateBack());

  wireSubmit(form, {
    button: document.getElementById('submit-btn'),
    errorBox: document.getElementById('error-box'),
    busyLabel: 'Creating…',
    failPrefix: `Failed to create ${cfg.kind}`,
    validate: () => {
      const invalid = resourceNameError(nameInput.value.trim(), cfg.nameMaxLength);
      if (invalid) {
        nameInput.focus();
        return invalid;
      }
      return fieldsError(cfg, form);
    },
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
      navigateToDetail(name);
    },
  });
}
