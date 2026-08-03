import { loadSdk, escapeHtml, emptyRow, errorRow } from './_shared.js';

await loadSdk();
await fundament.init;

const tbody = document.getElementById('rows');

// Computes the phase badge CSS class.
function phaseClass(phase) {
  if (phase === 'Ready') return 'ready';
  if (phase === 'Pending') return 'pending';
  if (phase === 'Failed') return 'failed';
  return 'unknown';
}

try {
  const { items } = await fundament.k8s.list({
    group: 'storage.fundament.io',
    version: 'v1alpha1',
    resource: 'storagepools',
  });

  if (!items || items.length === 0) {
    tbody.innerHTML = emptyRow(5, 'No storage pools.');
  } else {
    const rows = [];
    for (const item of items) {
      const meta = item.metadata ?? {};
      const status = item.status ?? {};
      const name = meta.name ?? '(unknown)';

      rows.push(`
        <tr style="cursor: pointer;" onclick="window.parent.postMessage({ type: 'plugin:navigate', name: '${escapeHtml(name)}' }, '*')">
          <td>${escapeHtml(name)}</td>
          <td><span class="plugin-badge" data-phase="${escapeHtml(phaseClass(status.phase))}">${escapeHtml(status.phase ?? 'Unknown')}</span></td>
          <td>${escapeHtml(status.storageClassName ?? '—')}</td>
          <td>${escapeHtml(String(status.replicas ?? '—'))}</td>
          <td>${escapeHtml(status.failureDomain ?? '—')}</td>
        </tr>`);
    }
    tbody.innerHTML = rows.join('');
  }
} catch (err) {
  tbody.innerHTML = errorRow(5, err);
}
