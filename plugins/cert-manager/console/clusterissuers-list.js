      import {
        loadSdk,
        escapeHtml,
        formatAge,
        readyText,
        issuerType,
        emptyRow,
        errorRow,
        navigateToDetail,
      } from './_shared.js';

      await loadSdk();
      await fundament.init;
      const tbody = document.getElementById('rows');

      try {
        const { items } = await fundament.k8s.list({
          group: 'cert-manager.io',
          version: 'v1',
          resource: 'clusterissuers',
        });
        if (!items || items.length === 0) {
          tbody.innerHTML = emptyRow(4, 'No cluster issuers found.');
        } else {
          tbody.innerHTML = items
            .map((item) => {
              const name = item.metadata?.name ?? '';
              return `
                <tr data-name="${escapeHtml(name)}">
                  <td><a href="#" class="row-link">${escapeHtml(name)}</a></td>
                  <td>${escapeHtml(readyText(item))}</td>
                  <td>${escapeHtml(issuerType(item))}</td>
                  <td>${escapeHtml(formatAge(item.metadata?.creationTimestamp))}</td>
                </tr>`;
            })
            .join('');
          tbody.querySelectorAll('a.row-link').forEach((link) => {
            link.addEventListener('click', (e) => {
              e.preventDefault();
              const row = link.closest('tr');
              navigateToDetail(row.dataset.name);
            });
          });
        }
      } catch (err) {
        tbody.innerHTML = errorRow(4, err);
      }
