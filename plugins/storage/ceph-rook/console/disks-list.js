import {
  bootPage,
  emptyStateHtml,
  errorBox,
  humanizeQuantity,
  loadingHtml,
  navigateToDetail,
  resourceListHtml,
  wireListRows,
} from './_shared.js';

const list = document.getElementById('list');
list.innerHTML = loadingHtml('Loading disks…');

await bootPage();

// Deliberately never says "Available". `available` is a stale-able snapshot of
// "empty and unformatted" — a disk running an OSD has been observed reporting
// available=true — so it cannot back a claim that a disk is free to take. Only
// two things here are facts: the claim this plugin recorded, and a filesystem
// the node actually saw. An absent filesystem is absence of evidence, so it
// stays unstated.
function claimText(status) {
  if (status?.claimedBy) return `Claimed by ${status.claimedBy}`;
  if (status?.filesystem) return `No claim recorded, contains ${status.filesystem}`;
  return 'No claim recorded';
}

// Soft-deleted disks are never removed (repo policy is soft deletes only), so a
// disk that has left discovery lingers as a Disk CR with available=false. Hide
// the fully-dead ones -- unavailable AND unclaimed -- so the list is not
// cluttered with departed or filtered-out devices. A disk a pool still claims
// stays visible even when unavailable, so it is never silently dropped.
function isLive(status) {
  return status?.available === true || Boolean(status?.claimedBy);
}

try {
  const { items } = await fundament.k8s.list({
    group: 'ceph.fundament.io',
    version: 'v1alpha1',
    resource: 'disks',
  });

  const live = (items ?? []).filter((item) => isLive(item.status ?? {}));

  if (live.length === 0) {
    list.innerHTML = emptyStateHtml({
      icon: 'ssd-hard-drive',
      text: 'No disks discovered yet',
      supportingText: 'Disks appear here once a node reports them.',
    });
  } else {
    // The device path first: it is what an operator recognizes from the node,
    // where the Disk's own name is a digest. Grouped by node through the sort.
    const byNode = (a, b) =>
      (a.status?.nodeName ?? '').localeCompare(b.status?.nodeName ?? '')
      || (a.status?.path ?? '').localeCompare(b.status?.path ?? '');
    list.innerHTML = resourceListHtml(
      [...live].sort(byNode).map((item) => {
        const s = item.status ?? {};
        const name = item.metadata?.name ?? '';
        const facts = [s.nodeName ?? '(unknown node)', humanizeQuantity(s.size), s.type].filter(Boolean);
        return {
          name,
          text: s.path ?? name,
          supportingText: `${facts.join(' · ')} · ${claimText(s)}`,
        };
      }),
      'Disks',
    );
    wireListRows(list, navigateToDetail);
  }
} catch (err) {
  list.innerHTML = errorBox(err);
}
