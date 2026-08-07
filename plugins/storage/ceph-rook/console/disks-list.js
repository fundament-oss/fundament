import { loadSdk, escapeHtml, humanizeBytes, emptyRow, errorRow } from './_shared.js';

await loadSdk();
await fundament.init;

const tbody = document.getElementById('rows');

// Computes the availability badge text for a disk item.
function availabilityText(status) {
  if (status?.claimedBy) return `Claimed by ${status.claimedBy}`;
  if (status?.available) return 'Available';
  return 'In use';
}

// Computes an availability CSS class suffix for styling (future use).
function availabilityClass(status) {
  if (status?.claimedBy) return 'claimed';
  if (status?.available) return 'available';
  return 'inuse';
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
    // Group disks by status.node, preserving insertion order.
    const byNode = new Map();
    for (const item of items) {
      const node = item.status?.node ?? '(unknown node)';
      if (!byNode.has(node)) byNode.set(node, []);
      byNode.get(node).push(item);
    }

    const rows = [];
    for (const [node, disks] of byNode) {
      // Emit a group header row spanning all columns.
      rows.push(
        `<tr><td colspan="5" class="plugin-text" style="font-weight:600;padding-top:0.75rem">${escapeHtml(node)}</td></tr>`,
      );
      for (const item of disks) {
        const s = item.status ?? {};
        rows.push(`
          <tr>
            <td></td>
            <td>${escapeHtml(s.path ?? '')}</td>
            <td>${escapeHtml(humanizeBytes(s.sizeBytes ?? 0))}</td>
            <td>${escapeHtml(s.type ?? '')}</td>
            <td data-availability="${escapeHtml(availabilityClass(s))}">${escapeHtml(availabilityText(s))}</td>
          </tr>`);
      }
    }
    tbody.innerHTML = rows.join('');
  }
} catch (err) {
  tbody.innerHTML = errorRow(5, err);
}
