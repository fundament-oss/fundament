import { loadSdk, escapeHtml, humanizeBytes, renderDefList, navigateBack } from './_shared.js';
import { selectableDisks, renderDiskPicker, readSelectedDisks } from './disk-picker.js';

await loadSdk();
const ctx = await fundament.init;

const content = document.getElementById('content');
const heading = document.getElementById('heading');
const actions = document.getElementById('actions');

const RESOURCE = {
  group: 'storage.fundament.io',
  version: 'v1alpha1',
  resource: 'storagepools',
};

const RESOURCE_DISKS = {
  group: 'storage.fundament.io',
  version: 'v1alpha1',
  resource: 'disks',
};

const RESOURCE_PVS = { version: 'v1', resource: 'persistentvolumes' };

const name = ctx.resource?.name;

function renderReadOnly(item) {
  const status = item.status ?? {};
  const spec = item.spec ?? {};

  const pairs = [
    ['Phase', status.phase ?? 'Unknown'],
    ['Storage Class', status.storageClassName ?? '—'],
    ['Replicas', String(status.replicas ?? '—')],
    ['Failure Domain', status.failureDomain ?? '—'],
    // Labelled as contributions, not capacity: the obvious reading is wrong.
    ['Disks contributed', String(status.selectedDiskCount ?? '—')],
    ['Raw size of contributed disks', humanizeBytes(status.rawCapacityBytes ?? 0)],
  ];
  if (status.message) pairs.push(['Message', status.message]);

  const disks =
    spec.disks && spec.disks.length > 0
      ? `<ul>${spec.disks.map((d) => `<li>${escapeHtml(d)}</li>`).join('')}</ul>`
      : '<p class="plugin-text">No disks selected.</p>';

  return `
    <h2 class="plugin-heading">Status</h2>
    ${renderDefList(pairs)}
    <p class="plugin-hint">
      Every storage pool feeds one shared Ceph cluster. Volumes provisioned through this
      pool are placed across all of the cluster's disks, not only the ones listed below,
      so the raw size above is this pool's contribution rather than its capacity. Use
      <code>ceph df</code> for actual free space.
    </p>
    <h2 class="plugin-heading">Contributed Disks</h2>
    ${disks}
  `;
}

async function showDetail() {
  try {
    const item = await fundament.k8s.get({ ...RESOURCE, name });
    heading.textContent = `Storage Pool · ${item.metadata?.name ?? name}`;
    content.innerHTML = renderReadOnly(item);
    actions.hidden = false;
    // .onclick, not addEventListener: these buttons live outside #content and
    // survive every re-render, so listeners would stack. (CSP restricts inline
    // handler *attributes*, not this.)
    document.getElementById('edit-btn').onclick = () => showEdit(item);
    document.getElementById('delete-btn').onclick = () => showDelete(item);
  } catch (err) {
    actions.hidden = true;
    content.innerHTML = `<div class="plugin-error">${escapeHtml(
      `Failed to load: ${err?.message ?? err}`,
    )}</div>`;
  }
}

async function showEdit(item) {
  actions.hidden = true;
  const current = item.spec?.disks ?? [];

  let disks;
  try {
    const { items } = await fundament.k8s.list(RESOURCE_DISKS);
    disks = selectableDisks(items, name);
  } catch (err) {
    content.innerHTML = `<div class="plugin-error">${escapeHtml(
      `Failed to load disks: ${err?.message ?? err}`,
    )}</div>`;
    actions.hidden = false;
    return;
  }

  const replication = item.spec?.replication ?? 'auto';
  const options = ['auto', '1', '2', '3']
    .map(
      (v) =>
        `<option value="${v}"${v === replication ? ' selected' : ''}>${
          v === 'auto' ? 'auto (recommended)' : v
        }</option>`,
    )
    .join('');

  content.innerHTML = `
    <form id="edit-form" class="plugin-form" novalidate>
      <div class="plugin-error" id="edit-error" hidden></div>

      <div class="plugin-field">
        <label class="plugin-label" for="replication">Replication</label>
        <select id="replication" name="replication" class="plugin-select">${options}</select>
        <span class="plugin-hint">auto derives the replica count from the number of nodes contributing disks to the cluster.</span>
      </div>

      <div class="plugin-field">
        <span class="plugin-label">Disks</span>
        ${renderDiskPicker(disks, current)}
        <span class="plugin-hint">
          Unchecking a disk removes it from the CephCluster device list but does NOT retire
          its OSD — that needs a manual Ceph purge, and data may rebalance.
        </span>
      </div>

      <div class="plugin-actions">
        <button type="submit" class="plugin-button" id="save-btn">Save</button>
        <button type="button" class="plugin-button-secondary" id="cancel-btn">Cancel</button>
      </div>
    </form>
  `;

  const form = document.getElementById('edit-form');
  const errorBox = document.getElementById('edit-error');
  const saveBtn = document.getElementById('save-btn');

  document.getElementById('cancel-btn').addEventListener('click', () => showDetail());

  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    errorBox.hidden = true;

    const selected = readSelectedDisks(form);
    if (selected.length === 0) {
      errorBox.textContent = 'Select at least one disk.';
      errorBox.hidden = false;
      return;
    }

    saveBtn.disabled = true;
    saveBtn.textContent = 'Saving…';
    try {
      // Merge-patch of spec only: status is untouched and disks is replaced
      // wholesale, not merged element-wise.
      await fundament.k8s.patch(
        { ...RESOURCE, name },
        {
          spec: {
            disks: selected,
            replication: form.querySelector('[name="replication"]').value,
          },
        },
      );
      await showDetail();
    } catch (err) {
      errorBox.textContent = `Failed to save: ${err?.message ?? err}`;
      errorBox.hidden = false;
      saveBtn.disabled = false;
      saveBtn.textContent = 'Save';
    }
  });
}

// Volumes still bound to this pool's StorageClass. Deleting the pool cascades to
// it via owner refs, so this has to be answered before offering the button.
async function boundVolumes(storageClassName) {
  if (!storageClassName) return [];
  const { items } = await fundament.k8s.list(RESOURCE_PVS);
  return (items ?? []).filter((pv) => pv.spec?.storageClassName === storageClassName);
}

async function showDelete(item) {
  actions.hidden = true;
  const sc = item.status?.storageClassName;

  let blocking;
  try {
    blocking = await boundVolumes(sc);
  } catch (err) {
    content.innerHTML = `<div class="plugin-error">${escapeHtml(
      `Could not check for bound volumes: ${err?.message ?? err}`,
    )}</div>`;
    actions.hidden = false;
    return;
  }

  if (blocking.length > 0) {
    const rows = blocking
      .map(
        (pv) => `
          <tr>
            <td>${escapeHtml(pv.metadata?.name ?? '')}</td>
            <td>${escapeHtml(pv.spec?.capacity?.storage ?? '—')}</td>
            <td>${escapeHtml(pv.status?.phase ?? '—')}</td>
            <td>${escapeHtml(
              pv.spec?.claimRef ? `${pv.spec.claimRef.namespace}/${pv.spec.claimRef.name}` : '—',
            )}</td>
          </tr>`,
      )
      .join('');
    content.innerHTML = `
      <div class="plugin-error">
        Cannot delete: ${blocking.length} volume(s) still use ${escapeHtml(sc)}.
        Delete those PersistentVolumeClaims first.
      </div>
      <table class="plugin-table">
        <thead><tr><th>Volume</th><th>Capacity</th><th>Phase</th><th>Claim</th></tr></thead>
        <tbody>${rows}</tbody>
      </table>
      <div class="plugin-actions">
        <button type="button" class="plugin-button-secondary" id="close-btn">Close</button>
      </div>`;
    document.getElementById('close-btn').addEventListener('click', () => showDetail());
    return;
  }

  content.innerHTML = `
    <h2 class="plugin-heading">Delete storage pool</h2>
    <p class="plugin-text">
      This also deletes the CephBlockPool and the StorageClass
      <strong>${escapeHtml(sc ?? '—')}</strong>. OSDs keep running until they are purged
      from Ceph manually.
    </p>
    <form id="delete-form" class="plugin-form" novalidate>
      <div class="plugin-error" id="delete-error" hidden></div>
      <div class="plugin-field">
        <label class="plugin-label" for="confirm">
          Type <strong>${escapeHtml(name)}</strong> to confirm
        </label>
        <input id="confirm" name="confirm" type="text" class="plugin-input" autocomplete="off" />
      </div>
      <div class="plugin-actions">
        <button type="submit" class="plugin-button" id="confirm-btn">Delete</button>
        <button type="button" class="plugin-button-secondary" id="abort-btn">Cancel</button>
      </div>
    </form>`;

  const form = document.getElementById('delete-form');
  const errorBox = document.getElementById('delete-error');
  document.getElementById('abort-btn').addEventListener('click', () => showDetail());

  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    errorBox.hidden = true;
    if (form.querySelector('[name="confirm"]').value.trim() !== name) {
      errorBox.textContent = 'The name does not match.';
      errorBox.hidden = false;
      return;
    }
    try {
      await fundament.k8s.delete({ ...RESOURCE, name });
      navigateBack();
    } catch (err) {
      errorBox.textContent = `Failed to delete: ${err?.message ?? err}`;
      errorBox.hidden = false;
    }
  });
}

if (!name) {
  content.textContent = 'No storage pool selected.';
} else {
  await showDetail();
}
