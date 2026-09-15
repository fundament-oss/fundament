// Page factory for the consumer kinds (BlockStorage, FileStorage): one
// implementation of the list/detail/create flows, parameterized per kind. The
// kinds differ only in labels/texts and the FileStorage-only metadataServers
// field, so each page file reduces to a factory call with its kind's config.

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
  replicationFieldHtml,
  metadataServersFieldHtml,
  metadataServersError,
  resourceNameError,
  wireSubmit,
} from './_shared.js';

const GROUP_VERSION = { group: 'storage.fundament.io', version: 'v1alpha1' };

export const BLOCKSTORAGE = {
  resource: 'blockstorages',
  kind: 'BlockStorage',
  label: 'Block Storage',
  emptyMessage: 'No block storage.',
  noneSelected: 'No block storage selected.',
  storageClassPrefix: 'ceph-',
  detailHint: "Volumes are placed across all of the shared Ceph cluster's disks.",
  createIntro: `Block storage provides ReadWriteOnce volumes over the shared Ceph cluster's disks.
    It needs at least one StoragePool contributing disks; without one it stays Degraded.`,
  metadataServers: false,
};

export const FILESTORAGE = {
  resource: 'filestorages',
  kind: 'FileStorage',
  label: 'File Storage',
  emptyMessage: 'No file storage.',
  noneSelected: 'No file storage selected.',
  storageClassPrefix: 'cephfs-',
  detailHint: 'Volumes can be mounted by many pods across nodes.',
  createIntro: `File storage provides shared ReadWriteMany volumes over the shared Ceph cluster's disks —
    many pods on many nodes can mount the same volume. It needs at least one StoragePool
    contributing disks; without one it stays Degraded.`,
  metadataServers: true,
};

function metadataServersValue(form) {
  return Number(form.querySelector('[name="metadataServers"]').value);
}

// specFrom reads the create/edit form into a spec object.
function specFrom(cfg, form) {
  const spec = { replication: form.querySelector('[name="replication"]').value };
  if (cfg.metadataServers) spec.metadataServers = metadataServersValue(form);
  return spec;
}

export async function consumerListPage(cfg) {
  await loadSdk();
  await fundament.init;

  const tbody = document.getElementById('rows');
  const colspan = cfg.metadataServers ? 7 : 6;

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
        return `
          <tr data-name="${escapeHtml(name)}">
            <td><a href="#" class="row-link">${escapeHtml(name)}</a></td>
            <td>${escapeHtml(status.phase ?? 'Unknown')}</td>
            <td>${escapeHtml(status.storageClassName ?? '—')}</td>
            <td>${escapeHtml(String(status.replicas ?? '—'))}</td>
            ${metadataServersCell}
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
    pairs.push(['Failure Domain', status.failureDomain ?? '—']);
    if (status.message) pairs.push(['Message', status.message]);

    return `
      <h2 class="plugin-heading">Status</h2>
      ${renderDefList(pairs)}
      <p class="plugin-hint">
        ${cfg.detailHint} Use <code>ceph df</code> for actual free space.
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

        ${replicationFieldHtml(item.spec?.replication ?? 'auto')}

        ${cfg.metadataServers ? metadataServersFieldHtml(item.spec?.metadataServers ?? 1) : ''}

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
      validate: cfg.metadataServers ? () => metadataServersError(metadataServersValue(form)) : undefined,
      action: async () => {
        // Merge-patch of spec only: status is untouched.
        await fundament.k8s.patch({ ...resource, name }, { spec: specFrom(cfg, form) });
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
               pattern="[a-z0-9]([a-z0-9\\-]*[a-z0-9])?" maxlength="63" />
        <span class="plugin-hint">Lowercase letters, digits and dashes. Names the resulting StorageClass (prefixed ${cfg.storageClassPrefix}).</span>
      </div>

      ${replicationFieldHtml()}

      ${cfg.metadataServers ? metadataServersFieldHtml() : ''}

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
      const invalid = resourceNameError(nameInput.value.trim());
      if (invalid) {
        nameInput.focus();
        return invalid;
      }
      return cfg.metadataServers ? metadataServersError(metadataServersValue(form)) : null;
    },
    action: async () => {
      const name = nameInput.value.trim();
      await fundament.k8s.create(
        { ...GROUP_VERSION, resource: cfg.resource },
        {
          apiVersion: 'storage.fundament.io/v1alpha1',
          kind: cfg.kind,
          metadata: { name },
          spec: specFrom(cfg, form),
        },
      );
      navigateToDetail(name);
    },
  });
}
