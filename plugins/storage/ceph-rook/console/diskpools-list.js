import {
  bootPage,
  errorBox,
  emptyStateHtml,
  humanizeQuantity,
  latestRead,
  loadingHtml,
  navigateToDetail,
  phaseBadgeHtml,
  resourceListHtml,
  selectedDiskCount,
  wireListRows,
} from './_shared.js';

const list = document.getElementById('list');
list.innerHTML = loadingHtml('Loading disk pools…');

await bootPage();

// The create form lives on sheet.html (sheet.js, then diskpool-forms.js),
// which the console opens in a sheet over this list.
document.getElementById('create-btn').addEventListener('click', () => {
  fundament.openSheet({ view: 'create' });
});

// "2 disks · 1.8 TiB", and why a pool is not Ready when it says so. A pool
// the operator has not counted yet has no count: that is not "0 disks".
function summary(status) {
  const count = selectedDiskCount(status);
  const parts = [];
  if (count !== undefined && count !== null) parts.push(`${count} ${count === 1 ? 'disk' : 'disks'}`);
  if (status.rawCapacity) parts.push(humanizeQuantity(status.rawCapacity));
  if (status.message) parts.push(status.message);
  return parts.join(' · ') || '—';
}

const nextRead = latestRead();

// Fills the list; again whenever a sheet over the list saved.
async function renderList() {
  const isCurrent = nextRead();
  try {
    const { items } = await fundament.k8s.list({
      group: 'ceph.fundament.io',
      version: 'v1alpha1',
      resource: 'diskpools',
    });
    if (!isCurrent()) return;

    if (!items || items.length === 0) {
      list.innerHTML = emptyStateHtml({
        icon: 'boxes-3',
        text: 'No disk pools yet',
        supportingText: 'A disk pool hands disks to the shared Ceph cluster. Create one to start.',
      });
      return;
    }
    list.innerHTML = resourceListHtml(
      items.map((item) => {
        const status = item.status ?? {};
        return {
          name: item.metadata?.name ?? '',
          supportingText: summary(status),
          badge: phaseBadgeHtml(status.phase),
        };
      }),
      'Disk pools',
    );
    wireListRows(list, navigateToDetail);
  } catch (err) {
    if (!isCurrent()) return;
    list.innerHTML = errorBox(err);
  }
}

fundament.onResourcesChanged(() => renderList());
await renderList();
