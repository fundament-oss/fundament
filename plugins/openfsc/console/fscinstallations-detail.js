      import {
        loadSdk,
        escapeHtml,
        renderDefList,
        renderConditionsTable,
        renderGatewayTable,
      } from './_shared.js';

      const content = document.getElementById('content');
      const heading = document.getElementById('heading');

      let ctx;
      try {
        await loadSdk();
        ctx = await fundament.init;
      } catch (err) {
        content.textContent = `Failed to load the plugin SDK: ${err?.message ?? err}`;
        ctx = null;
      }

      if (ctx && !ctx.resource?.name) {
        content.textContent = 'No FSC installation selected.';
      } else if (ctx) {
        try {
          const item = await fundament.k8s.get({
            group: 'openfsc.fundament.io',
            version: 'v1',
            resource: 'fscinstallations',
            name: ctx.resource.name,
            namespace: ctx.resource.namespace,
          });
          heading.textContent = `FSC installation · ${item.metadata?.name ?? ctx.resource.name}`;

          const directory = item.spec?.directory ?? {};
          const spec = {
            Namespace: item.metadata?.namespace,
            'Group ID': item.spec?.groupID,
            'Peer ID': item.spec?.peerID,
            'Directory mode': directory.mode,
            ...(directory.external && {
              'Directory address': directory.external.address,
              'Directory peer ID': directory.external.peerID,
            }),
            Created: item.metadata?.creationTimestamp,
          };
          const status = {
            Phase: item.status?.phase ?? '—',
            Message: item.status?.message,
            'Manager address': item.status?.managerAddress,
          };

          // Only link http(s) URLs: anything else (javascript:, data:) would
          // execute when clicked.
          const controllerURL = item.status?.controllerURL;
          const controllerLink = /^https?:\/\//.test(controllerURL ?? '')
            ? `<p class="plugin-text">Controller UI:
                 <a href="${escapeHtml(controllerURL)}" target="_blank" rel="noopener">${escapeHtml(controllerURL)}</a>
               </p>`
            : '';

          content.innerHTML = `
            <h2 class="plugin-heading">Spec</h2>
            ${renderDefList(spec)}
            <h2 class="plugin-heading">Status</h2>
            ${renderDefList(status)}
            ${controllerLink}
            <h2 class="plugin-heading">Inways</h2>
            ${renderGatewayTable(item.status?.inways)}
            <h2 class="plugin-heading">Outways</h2>
            ${renderGatewayTable(item.status?.outways)}
            <h2 class="plugin-heading">Conditions</h2>
            ${renderConditionsTable(item)}`;
        } catch (err) {
          content.textContent = `Failed to load: ${err?.message ?? err}`;
        }
      }
