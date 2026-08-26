import {
  loadSdk,
  escapeHtml,
  formatAge,
  readyText,
  emptyRow,
  errorRow,
  navigateToDetail,
} from './_shared.js';

await loadSdk();
await fundament.init;
const tbody = document.getElementById('rows');

try {
  const { items } = await fundament.k8s.list({
    group: 'example.com',
    version: 'v1',
    resource: 'widgets',
  });
  if (!items || items.length === 0) {
    tbody.innerHTML = emptyRow(4, 'No widgets found.');
  } else {
    tbody.innerHTML = items
      .map((item) => {
        const name = item.metadata?.name ?? '';
        const namespace = item.metadata?.namespace ?? '';
        return `
          <tr data-name="${escapeHtml(name)}" data-namespace="${escapeHtml(namespace)}">
            <td><a href="#" class="row-link">${escapeHtml(name)}</a></td>
            <td>${escapeHtml(namespace)}</td>
            <td>${escapeHtml(readyText(item))}</td>
            <td>${escapeHtml(formatAge(item.metadata?.creationTimestamp))}</td>
          </tr>`;
      })
      .join('');
    tbody.querySelectorAll('a.row-link').forEach((link) => {
      link.addEventListener('click', (e) => {
        e.preventDefault();
        const row = link.closest('tr');
        navigateToDetail(row.dataset.name, row.dataset.namespace || undefined);
      });
    });
  }
} catch (err) {
  tbody.innerHTML = errorRow(4, err);
}
