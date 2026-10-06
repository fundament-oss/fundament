import {
  loadSdk,
  openSheet,
  errorBox,
  escapeHtml,
  humanizeQuantity,
  renderDefList,
  wireSubmit,
} from './_shared.js';
import { selectableDisks, renderDiskPicker, readSelectedDisks } from './disk-picker.js';

await loadSdk();
const ctx = await fundament.init;

const content = document.getElementById('content');
const heading = document.getElementById('heading');
const actions = document.getElementById('actions');

const RESOURCE = {
  group: 'ceph.fundament.io',
  version: 'v1alpha1',
  resource: 'diskpools',
};

const RESOURCE_DISKS = {
  group: 'ceph.fundament.io',
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

// spec.disks entries name Disk CRs — a node prefix plus a digest of the device's
// stable identity — which match nothing an operator sees on the node or in the
// Disks list. Resolve each to its device path and keep the CR name underneath,
// so this page and the Disks list can be compared by eye. A name with no Disk CR
// behind it has no path to show and says so rather than rendering bare.
function renderDiskList(entries, byName) {
  const names = (entries ?? []).map((d) => d.name);
  if (names.length === 0) return '<p class="plugin-text">No disks selected.</p>';
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
    // Labelled as contributions, not capacity: the obvious reading is wrong.
    ['Disks contributed', String(status.selectedDiskCount ?? '—')],
    ['Raw size of contributed disks', humanizeQuantity(status.rawCapacity)],
  ];
  if (status.message) pairs.push(['Message', status.message]);

  return `
    <h2 class="plugin-heading">Status</h2>
    ${renderDefList(pairs)}
    <p class="plugin-hint">
      Every disk pool feeds one shared Ceph cluster; BlockStorage and FileStorage objects
      turn that capacity into StorageClasses. Volumes provisioned through those StorageClasses
      are placed across all of the cluster's disks, so the raw size above is this pool's
      contribution. Use
      <code>ceph df</code> for free space.
    </p>
    <h2 class="plugin-heading">Contributed Disks</h2>
    ${renderDiskList(spec.disks, byName)}
  `;
}

async function showDetail() {
  try {
    const item = await fundament.k8s.get({ ...RESOURCE, name });
    heading.textContent = `Disk Pool · ${item.metadata?.name ?? name}`;
    content.innerHTML = renderReadOnly(item, await diskIndex());
    actions.hidden = false;
    // .onclick, not addEventListener: this button lives outside #content and
    // survives every re-render, so listeners would stack. (CSP restricts inline
    // handler *attributes*, not this.)
    document.getElementById('edit-btn').onclick = () => showEdit(item);
  } catch (err) {
    actions.hidden = true;
    content.innerHTML = errorBox(err);
  }
}

async function showEdit(item) {
  const sheet = await openSheet({ label: 'Edit Disk Pool' });
  if (!sheet) return;
  const { body, close } = sheet;
  body.insertAdjacentHTML('beforeend', '<p class="plugin-text">Loading disks…</p>');
  const current = item.spec?.disks ?? [];
  const currentNames = current.map((d) => d.name);

  let disks;
  try {
    const { items } = await fundament.k8s.list(RESOURCE_DISKS);
    disks = selectableDisks(items, name);
  } catch (err) {
    body.lastElementChild.outerHTML = errorBox(err, 'Failed to load disks');
    return;
  }

  // The picker can only offer disks it can see: one claimed by another pool, or
  // named in spec with no Disk CR behind it, never gets a checkbox. Save rebuilds
  // spec.disks wholesale, so anything the form did not render used to disappear
  // on a save the operator did not intend as a change — one click after the
  // detail page flagged that exact disk. Carry them through untouched, and name
  // them below rather than holding them silently.
  const rendered = new Set(disks.map((d) => d.metadata?.name).filter(Boolean));
  const preserved = current.filter((d) => !rendered.has(d.name));
  const preservedNote =
    preserved.length === 0
      ? ''
      : `<p class="plugin-hint">
           Kept as they are, because this form cannot show them: a disk is listed here only
           when it exists and is either free or already claimed by this pool:
           ${escapeHtml(preserved.map((d) => d.name).join(', '))}. Saving leaves them in the pool; use kubectl
           to remove one.
         </p>`;

  body.lastElementChild.outerHTML = `
    <form class="plugin-form" novalidate>
      <div class="plugin-error" data-role="error" hidden></div>

      <div class="plugin-field">
        <span class="plugin-label">Disks</span>
        ${renderDiskPicker(disks, currentNames)}
        ${preservedNote}
        <span class="plugin-hint">
          Unchecking a disk removes it from the shared Ceph cluster's device list, but its
          storage daemon (OSD) keeps running until it is purged from Ceph manually, and
          data may rebalance in the meantime.
        </span>
      </div>

      <div class="plugin-actions">
        <button type="submit" class="plugin-button" data-role="save">Save</button>
        <button type="button" class="plugin-button-secondary" data-role="cancel">Cancel</button>
      </div>
    </form>
  `;

  const form = body.querySelector('form');

  body.querySelector('[data-role="cancel"]').addEventListener('click', () => close());

  // Disjoint by construction: preserved is exactly what the picker did not
  // render, so this cannot produce the duplicate name the CRD's listType=map
  // rejects.
  const selected = () => [...readSelectedDisks(form, current), ...preserved];

  wireSubmit(form, {
    button: body.querySelector('[data-role="save"]'),
    errorBox: body.querySelector('[data-role="error"]'),
    busyLabel: 'Saving…',
    failPrefix: 'Failed to save',
    validate: () => (selected().length === 0 ? 'Select at least one disk.' : null),
    action: async () => {
      // Merge-patch of spec only: status is untouched and disks is replaced
      // wholesale, not merged element-wise.
      await fundament.k8s.patch(
        { ...RESOURCE, name },
        {
          spec: { disks: selected() },
        },
      );
      close();
      await showDetail();
    },
  });
}

if (!name) {
  content.textContent = 'No disk pool selected.';
} else {
  await showDetail();
}
