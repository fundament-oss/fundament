import {
  bootPage,
  errorBox,
  emptyStateHtml,
  humanizeQuantity,
  loadingHtml,
  navigateToDetail,
  phaseBadgeHtml,
  resourceListHtml,
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

// "2 disks · 1.8 TiB", and why a pool is not Ready when it says so.
function summary(status) {
  const count = status.selectedDiskCount ?? 0;
  const parts = [`${count} ${count === 1 ? 'disk' : 'disks'}`, humanizeQuantity(status.rawCapacity)];
  if (status.message) parts.push(status.message);
  return parts.join(' · ');
}

// Fills the list; again whenever a sheet over the list saved.
async function renderList() {
  try {
    const { items } = await fundament.k8s.list({
      group: 'ceph.fundament.io',
      version: 'v1alpha1',
      resource: 'diskpools',
    });

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
    list.innerHTML = errorBox(err);
  }
}

fundament.onResourcesChanged(() => renderList());
await renderList();
