import {
  loadSdk,
  loadNldd,
  escapeHtml,
  formatAge,
  phase,
  emptyRow,
  errorRow,
  navigateToDetail,
  navigateToCreate,
} from './shared.ts';
import type { FSCInstallation } from './types.ts';

const tbody = document.getElementById('rows') as HTMLElement;
document.getElementById('add')!.addEventListener('click', () => navigateToCreate());

// Only the <nldd-button> needs the heavy NLDD Design System bundle (the rows are
// plain .plugin-* markup), so start it but don't block the data fetch on it — and
// don't let it fail the page. If it never arrives the table still renders and the
// "Add installation" button degrades to an unstyled unknown element, which beats
// replacing a good table with an error row.
loadNldd().catch((err) => {
  console.error('NLDD Design System failed to load; <nldd-*> components will not render.', err);
});

try {
  await loadSdk();
  await window.fundament.init;
  const { items } = await window.fundament.k8s.list<FSCInstallation>({
    group: 'openfsc.fundament.io',
    version: 'v1',
    resource: 'fscinstallations',
  });
  if (!items || items.length === 0) {
    tbody.innerHTML = emptyRow(6, 'No FSC installations found.');
  } else {
    tbody.innerHTML = items
      .map((item) => {
        const name = item.metadata?.name ?? '';
        const namespace = item.metadata?.namespace ?? '';
        return `
          <tr data-name="${escapeHtml(name)}" data-namespace="${escapeHtml(namespace)}">
            <td>${escapeHtml(namespace)}</td>
            <td><a href="#" class="row-link">${escapeHtml(name)}</a></td>
            <td>${escapeHtml(item.spec?.groupID ?? '')}</td>
            <td>${escapeHtml(item.spec?.directory?.mode ?? '')}</td>
            <td>${escapeHtml(phase(item))}</td>
            <td>${escapeHtml(formatAge(item.metadata?.creationTimestamp))}</td>
          </tr>`;
      })
      .join('');
    tbody.querySelectorAll<HTMLAnchorElement>('a.row-link').forEach((link) => {
      link.addEventListener('click', (e) => {
        e.preventDefault();
        const row = link.closest('tr') as HTMLElement;
        navigateToDetail(row.dataset.name, row.dataset.namespace);
      });
    });
  }
} catch (err) {
  tbody.innerHTML = errorRow(6, err);
}
