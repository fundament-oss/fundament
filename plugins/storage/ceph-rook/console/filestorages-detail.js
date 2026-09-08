import {
  loadSdk,
  escapeHtml,
  renderDefList,
  replicationFieldHtml,
  metadataServersFieldHtml,
  metadataServersError,
  wireSubmit,
} from './_shared.js';

await loadSdk();
const ctx = await fundament.init;

const content = document.getElementById('content');
const heading = document.getElementById('heading');
const actions = document.getElementById('actions');

const RESOURCE = {
  group: 'storage.fundament.io',
  version: 'v1alpha1',
  resource: 'filestorages',
};

const name = ctx.resource?.name;

function renderReadOnly(item) {
  const status = item.status ?? {};

  const pairs = [
    ['Phase', status.phase ?? 'Unknown'],
    ['Storage Class', status.storageClassName ?? '—'],
    ['Replicas', String(status.replicas ?? '—')],
    ['Metadata servers', String(item.spec?.metadataServers ?? 1)],
    ['Failure Domain', status.failureDomain ?? '—'],
  ];
  if (status.message) pairs.push(['Message', status.message]);

  return `
    <h2 class="plugin-heading">Status</h2>
    ${renderDefList(pairs)}
    <p class="plugin-hint">
      Volumes can be mounted by many pods across nodes. Use
      <code>ceph df</code> for actual free space.
    </p>
  `;
}

async function showDetail() {
  try {
    const item = await fundament.k8s.get({ ...RESOURCE, name });
    heading.textContent = `File Storage · ${item.metadata?.name ?? name}`;
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

      ${metadataServersFieldHtml(item.spec?.metadataServers ?? 1)}

      <div class="plugin-actions">
        <button type="submit" class="plugin-button" id="save-btn">Save</button>
        <button type="button" class="plugin-button-secondary" id="cancel-btn">Cancel</button>
      </div>
    </form>
  `;

  const form = document.getElementById('edit-form');
  const metadataServers = () => Number(form.querySelector('[name="metadataServers"]').value);

  document.getElementById('cancel-btn').addEventListener('click', () => showDetail());

  wireSubmit(form, {
    button: document.getElementById('save-btn'),
    errorBox: document.getElementById('edit-error'),
    busyLabel: 'Saving…',
    failPrefix: 'Failed to save',
    validate: () => metadataServersError(metadataServers()),
    action: async () => {
      // Merge-patch of spec only: status is untouched.
      await fundament.k8s.patch(
        { ...RESOURCE, name },
        {
          spec: {
            replication: form.querySelector('[name="replication"]').value,
            metadataServers: metadataServers(),
          },
        },
      );
      await showDetail();
    },
  });
}

if (!name) {
  content.textContent = 'No file storage selected.';
} else {
  await showDetail();
}
