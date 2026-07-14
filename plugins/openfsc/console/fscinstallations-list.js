      import {
        loadSdk,
        escapeHtml,
        formatAge,
        phase,
        emptyRow,
        errorRow,
        navigateToDetail,
        navigateToCreate,
      } from './_shared.js';

      const tbody = document.getElementById('rows');
      document.getElementById('add').addEventListener('click', () => navigateToCreate());

      try {
        await loadSdk();
        await fundament.init;
        const { items } = await fundament.k8s.list({
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
          tbody.querySelectorAll('a.row-link').forEach((link) => {
            link.addEventListener('click', (e) => {
              e.preventDefault();
              const row = link.closest('tr');
              navigateToDetail(row.dataset.name, row.dataset.namespace);
            });
          });
        }
      } catch (err) {
        tbody.innerHTML = errorRow(6, err);
      }
