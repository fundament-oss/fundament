import { loadSdk, escapeHtml, emptyRow, errorRow, wireRowLinks } from './_shared.js';

await loadSdk();
await fundament.init;

const tbody = document.getElementById('rows');

try {
  const { items } = await fundament.k8s.list({
    group: 'storage.fundament.io',
    version: 'v1alpha1',
    resource: 'storagepools',
  });

  if (!items || items.length === 0) {
    tbody.innerHTML = emptyRow(6, 'No storage pools.');
  } else {
    tbody.innerHTML = items
      .map((item) => {
        const status = item.status ?? {};
        const name = item.metadata?.name ?? '';
        return `
          <tr data-name="${escapeHtml(name)}">
            <td><a href="#" class="row-link">${escapeHtml(name)}</a></td>
            <td>${escapeHtml(status.phase ?? 'Unknown')}</td>
            <td>${escapeHtml(status.storageClassName ?? '—')}</td>
            <td>${escapeHtml(String(status.replicas ?? '—'))}</td>
            <td>${escapeHtml(status.failureDomain ?? '—')}</td>
            <td>${escapeHtml(status.message ?? '')}</td>
          </tr>`;
      })
      .join('');
    wireRowLinks(tbody);
  }
} catch (err) {
  tbody.innerHTML = errorRow(6, err);
}
