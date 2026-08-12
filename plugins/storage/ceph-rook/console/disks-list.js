import {
  loadSdk,
  escapeHtml,
  humanizeBytes,
  emptyRow,
  errorRow,
  wireRowLinks,
} from './_shared.js';

await loadSdk();
await fundament.init;

const tbody = document.getElementById('rows');

// A disk Ceph already consumes stops reporting as empty, so `available` alone
// cannot tell "in use by us" from "in use by something else". claimedBy is what
// distinguishes them.
function availabilityText(status) {
  if (status?.claimedBy) return `Claimed by ${status.claimedBy}`;
  if (status?.available) return 'Available';
  return 'In use';
}

try {
  const { items } = await fundament.k8s.list({
    group: 'storage.fundament.io',
    version: 'v1alpha1',
    resource: 'disks',
  });

  if (!items || items.length === 0) {
    tbody.innerHTML = emptyRow(5, 'No disks discovered yet.');
  } else {
    tbody.innerHTML = items
      .map((item) => {
        const s = item.status ?? {};
        const name = item.metadata?.name ?? '';
        return `
          <tr data-name="${escapeHtml(name)}">
            <td>${escapeHtml(s.node ?? '(unknown node)')}</td>
            <td><a href="#" class="row-link">${escapeHtml(s.path ?? name)}</a></td>
            <td>${escapeHtml(humanizeBytes(s.sizeBytes ?? 0))}</td>
            <td>${escapeHtml(s.type ?? '')}</td>
            <td>${escapeHtml(availabilityText(s))}</td>
          </tr>`;
      })
      .join('');
    wireRowLinks(tbody);
  }
} catch (err) {
  tbody.innerHTML = errorRow(5, err);
}
