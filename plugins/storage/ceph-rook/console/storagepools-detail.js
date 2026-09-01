import { loadSdk, escapeHtml, humanizeBytes, renderDefList } from './_shared.js';
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

const name = ctx.resource?.name;

// Disk CRs keyed by name. Best-effort: without it the page falls back to raw CR
// names, which is worse but still readable, so a failed list must not take the
// whole detail page down.
async function diskIndex() {
  try {
    const { items } = await fundament.k8s.list(RESOURCE_DISKS);
    return new Map((items ?? []).map((d) => [d.metadata?.name, d]));
  } catch {
    return new Map();
  }
}

// spec.disks holds Disk CR names — a node prefix plus a digest of the device's
// stable identity — which match nothing an operator sees on the node or in the
// Disks list. Resolve each to its device path and keep the CR name underneath,
// so this page and the Disks list can be compared by eye. A name with no Disk CR
// behind it has no path to show and says so rather than rendering bare.
function renderDiskList(names, byName) {
  if (!names || names.length === 0) return '<p class="plugin-text">No disks selected.</p>';
  const rows = names
    .map((diskName) => {
      const path = byName.get(diskName)?.status?.path;
      const primary = path ?? diskName;
      const secondary = path ? diskName : 'no matching disk found';
      return `<li>${escapeHtml(primary)}<br /><span class="plugin-hint">${escapeHtml(
        secondary,
      )}</span></li>`;
    })
    .join('');
  return `<ul>${rows}</ul>`;
}

function renderReadOnly(item, byName) {
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
    ${renderDiskList(spec.disks, byName)}
  `;
}

async function showDetail() {
  try {
    const item = await fundament.k8s.get({ ...RESOURCE, name });
    heading.textContent = `Storage Pool · ${item.metadata?.name ?? name}`;
    content.innerHTML = renderReadOnly(item, await diskIndex());
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

  // The picker can only offer disks it can see: one claimed by another pool, or
  // named in spec with no Disk CR behind it, never gets a checkbox. Save rebuilds
  // spec.disks wholesale, so anything the form did not render used to disappear
  // on a save the operator did not intend as a change — one click after the
  // detail page flagged that exact disk. Carry them through untouched, and name
  // them below rather than holding them silently.
  const rendered = new Set(disks.map((d) => d.metadata?.name).filter(Boolean));
  const preserved = current.filter((d) => !rendered.has(d));
  const preservedNote =
    preserved.length === 0
      ? ''
      : `<p class="plugin-hint">
           Kept as they are, because this form cannot show them — a disk is only listed here
           when it exists and is either free or already claimed by this pool:
           ${escapeHtml(preserved.join(', '))}. Saving leaves them in the pool; use kubectl
           to remove one.
         </p>`;

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
        ${preservedNote}
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

    // Disjoint by construction: preserved is exactly what the picker did not
    // render, so this cannot produce the duplicate the CRD's listType=set rejects.
    const selected = [...readSelectedDisks(form), ...preserved];
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

if (!name) {
  content.textContent = 'No storage pool selected.';
} else {
  await showDetail();
}
