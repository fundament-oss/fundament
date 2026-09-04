import { loadSdk, escapeHtml, formatAge, phase, emptyRow, errorRow, navigateToDetail } from './shared.ts';
import type { Widget } from './types.ts';

const COLUMNS = 4;

const sdk = await loadSdk();
await sdk.init;
const tbody = document.getElementById('rows') as HTMLElement;

try {
  const { items } = await sdk.k8s.list<Widget>({
    group: 'example.com',
    version: 'v1',
    resource: 'widgets',
  });

  if (!items || items.length === 0) {
    tbody.innerHTML = emptyRow(COLUMNS, 'No widgets found.');
  } else {
    tbody.innerHTML = items
      .map((item) => {
        const name = item.metadata?.name ?? '';
        const namespace = item.metadata?.namespace ?? '';
        return `
          <tr data-name="${escapeHtml(name)}" data-namespace="${escapeHtml(namespace)}">
            <td><a href="#" class="row-link">${escapeHtml(name)}</a></td>
            <td>${escapeHtml(namespace)}</td>
            <td>${escapeHtml(phase(item))}</td>
            <td>${escapeHtml(formatAge(item.metadata?.creationTimestamp))}</td>
          </tr>`;
      })
      .join('');

    tbody.querySelectorAll<HTMLAnchorElement>('a.row-link').forEach((link) => {
      link.addEventListener('click', (e) => {
        e.preventDefault();
        const row = link.closest('tr') as HTMLTableRowElement;
        navigateToDetail(row.dataset.name, row.dataset.namespace || undefined);
      });
    });
  }
} catch (err) {
  tbody.innerHTML = errorRow(COLUMNS, err);
}
