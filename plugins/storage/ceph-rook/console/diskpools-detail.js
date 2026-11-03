import {
  loadSdk,
  errorBox,
  fetchCephCapacity,
  escapeHtml,
  humanizeQuantity,
  renderDefList,
} from './_shared.js';

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
      contribution.
    </p>
    <p class="plugin-hint" data-role="capacity"></p>
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
    // The edit form is diskpools-edit.html, in a console sheet.
    document.getElementById('edit-btn').onclick = () => {
      fundament.openSheet({ view: 'edit', resource: { name } });
    };
    fetchCephCapacity().then((cap) => {
      const el = content.querySelector('[data-role="capacity"]');
      if (!cap || !el) return;
      el.textContent = `Raw free space: ${humanizeQuantity(String(cap.bytesAvailable))} of `
        + `${humanizeQuantity(String(cap.bytesTotal))} across the shared cluster.`;
    });
  } catch (err) {
    actions.hidden = true;
    content.innerHTML = errorBox(err);
  }
}

if (!name) {
  content.textContent = 'No disk pool selected.';
} else {
  // The edit sheet saved: show what it saved.
  fundament.onResourcesChanged(() => showDetail());
  await showDetail();
}
