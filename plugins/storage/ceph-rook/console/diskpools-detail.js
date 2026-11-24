import {
  addInfoButton,
  bootPage,
  DISK_POOL_INFO,
  errorBox,
  fetchCephCapacity,
  escapeHtml,
  freeSpace,
  hintHtml,
  humanizeQuantity,
  latestRead,
  loadingHtml,
  renderKeyValueList,
  sectionHtml,
  selectedDiskCount,
} from './_shared.js';

const content = document.getElementById('content');
const actions = document.getElementById('actions');
content.innerHTML = loadingHtml();

const ctx = await bootPage();

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

// In the actions group, which shows once the pool has loaded.
addInfoButton(document.getElementById('page-actions'), 'About disk pools', [
  ...DISK_POOL_INFO,
  'Raw size is what this pool\'s disks add to the cluster, before copies are made.',
  'Ceph runs one storage daemon (OSD) per disk. Removing a disk from the pool does not stop its daemon: that is a manual step in Ceph, and data may move between disks in the meantime.',
]);

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
  if (names.length === 0) return hintHtml('No disks');
  const rows = names
    .map((diskName) => {
      const path = byName.get(diskName)?.status?.path;
      const primary = path ?? diskName;
      const secondary = path ? diskName : 'Disk not found';
      return `
        <nldd-list-item>
          <nldd-text-cell text="${escapeHtml(primary)}" supporting-text="${escapeHtml(secondary)}"></nldd-text-cell>
        </nldd-list-item>`;
    })
    .join('');
  return `<nldd-list appearance="simple" accessible-label="Disks">${rows}</nldd-list>`;
}

function renderReadOnly(item, byName, cap) {
  const status = item.status ?? {};
  const spec = item.spec ?? {};

  // The phase is the badge beside the Console's title; the message says why.
  const pairs = [
    ['Disks', String(selectedDiskCount(status) ?? '—')],
    // Raw, not capacity: copies take their share once storage is built on it.
    ['Raw size', humanizeQuantity(status.rawCapacity)],
  ];
  const free = freeSpace(cap, null);
  if (free) pairs.push(free);
  if (status.message) pairs.push(['Message', status.message]);

  return `
    ${sectionHtml('Status', renderKeyValueList(pairs, 'Status'))}
    <nldd-spacer size="24"></nldd-spacer>
    ${sectionHtml('Disks', renderDiskList(spec.disks, byName))}
  `;
}

const nextRead = latestRead();

async function showDetail() {
  const isCurrent = nextRead();
  try {
    // diskIndex and fetchCephCapacity never throw: without them the page
    // shows raw disk names and leaves out the free space row.
    const [item, byName, cap] = await Promise.all([
      fundament.k8s.get({ ...RESOURCE, name }),
      diskIndex(),
      fetchCephCapacity(),
    ]);
    if (!isCurrent()) return;
    content.innerHTML = renderReadOnly(item, byName, cap);
    actions.hidden = false;
    // .onclick, not addEventListener: this button lives outside #content and
    // survives every re-render, so listeners would stack. (CSP restricts inline
    // handler *attributes*, not this.)
    // The edit form lives on sheet.html (sheet.js, then diskpool-forms.js), in
    // a console sheet.
    document.getElementById('edit-btn').onclick = () => {
      fundament.openSheet({ view: 'edit', resource: { name } });
    };
  } catch (err) {
    if (!isCurrent()) return;
    actions.hidden = true;
    content.innerHTML = errorBox(err);
  }
}

if (!name) {
  content.innerHTML = errorBox('no disk pool selected', 'Cannot show the disk pool');
} else {
  // The edit sheet saved: show what it saved.
  fundament.onResourcesChanged(() => showDetail());
  await showDetail();
}
