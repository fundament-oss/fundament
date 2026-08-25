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

// Deliberately never says "Available". `available` is a stale-able snapshot of
// "empty and unformatted" — a disk running an OSD has been observed reporting
// available=true — so it cannot back a claim that a disk is free to take. Only
// two things here are facts: the claim this plugin recorded, and a filesystem
// the node actually saw. An absent filesystem is absence of evidence, so it
// stays unstated.
function claimText(status) {
  if (status?.claimedBy) return `Claimed by ${status.claimedBy}`;
  if (status?.filesystem) return `No claim recorded — contains ${status.filesystem}`;
  return 'No claim recorded';
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
            <td>${escapeHtml(claimText(s))}</td>
          </tr>`;
      })
      .join('');
    wireRowLinks(tbody);
  }
} catch (err) {
  tbody.innerHTML = errorRow(5, err);
}
