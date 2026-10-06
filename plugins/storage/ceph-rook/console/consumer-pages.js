// Page factory for the consumer kinds (BlockStorage, FileStorage): one
// implementation of the list/detail/create flows, parameterized per kind. The
// kinds differ only in labels/texts and the FileStorage-only metadataServers
// field, so each page file reduces to a factory call with its kind's config.

import {
  loadSdk,
  loadNlddDesignSystem,
  escapeHtml,
  emptyRow,
  errorRow,
  wireRowLinks,
  navigateToCreate,
  navigateToDetail,
  navigateBack,
  renderDefList,
  replicationFieldHtml,
  defaultFieldHtml,
  metadataServersFieldHtml,
  metadataServersError,
  resourceNameError,
  wireSubmit,
  formFieldHtml,
  formActionsHtml,
  errorBannerHtml,
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

function metadataServersValue(form) {
  return Number(form.querySelector('[name="metadataServers"]').value);
}

// The metadata-servers check, for the kinds that have the field.
function metadataServersChecks(cfg, form) {
  if (!cfg.metadataServers) return [];
  return [[form.querySelector('[name="metadataServers"]'), (value) => metadataServersError(Number(value))]];
}

// specFrom reads the create/edit form into a spec object.
function specFrom(cfg, form) {
  const spec = { replication: form.querySelector('[name="replication"]').value };
  if (cfg.metadataServers) spec.metadataServers = metadataServersValue(form);
  // Always sent, so unticking the box merge-patches the field back to false.
  if (cfg.defaultToggle) spec.default = form.querySelector('[name="default"]').checked;
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
  // The design system is for the edit form.
  await Promise.all([loadSdk(), loadNlddDesignSystem()]);
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
      <nldd-form>
        <form id="edit-form" novalidate>
          ${errorBannerHtml('edit-error')}

          ${replicationFieldHtml(item.spec?.replication ?? 'auto')}

          ${cfg.metadataServers ? metadataServersFieldHtml(item.spec?.metadataServers ?? 1) : ''}

          ${cfg.defaultToggle ? defaultFieldHtml(item.spec?.default === true) : ''}

          ${formActionsHtml({ submitId: 'save-btn', submitText: 'Save', cancelId: 'cancel-btn' })}
        </form>
      </nldd-form>
    `;

    const form = document.getElementById('edit-form');

    document.getElementById('cancel-btn').addEventListener('click', () => showDetail());

    wireSubmit(form, {
      button: document.getElementById('save-btn'),
      errorBanner: document.getElementById('edit-error'),
      failPrefix: 'Failed to save',
      checks: metadataServersChecks(cfg, form),
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
  await Promise.all([loadSdk(), loadNlddDesignSystem()]);
  await fundament.init;

  const content = document.getElementById('content');

  content.innerHTML = `
    <nldd-rich-text>
      <p>${cfg.createIntro}</p>
    </nldd-rich-text>
    <nldd-spacer size="12"></nldd-spacer>

    <nldd-form>
      <form id="create-form" novalidate>
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

        ${replicationFieldHtml()}

        ${cfg.metadataServers ? metadataServersFieldHtml() : ''}

        ${cfg.defaultToggle ? defaultFieldHtml() : ''}

        ${formActionsHtml({ submitId: 'submit-btn', submitText: `Create ${cfg.kind}`, cancelId: 'cancel-btn' })}
      </form>
    </nldd-form>
  `;

  const form = document.getElementById('create-form');
  const nameInput = form.querySelector('[name="name"]');

  document.getElementById('cancel-btn').addEventListener('click', () => navigateBack());

  wireSubmit(form, {
    button: document.getElementById('submit-btn'),
    errorBanner: document.getElementById('error-banner'),
    failPrefix: `Failed to create ${cfg.kind}`,
    checks: [
      [nameInput, (value) => resourceNameError(value, cfg.nameMaxLength)],
      ...metadataServersChecks(cfg, form),
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
      navigateToDetail(name);
    },
  });
}
