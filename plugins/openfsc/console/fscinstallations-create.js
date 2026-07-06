      import { loadSdk, escapeHtml, navigateToDetail, navigateBack } from './_shared.js';

      const intro = document.getElementById('intro');
      const form = document.getElementById('form');
      const errorBox = document.getElementById('error');
      const externalFieldset = document.getElementById('external-fieldset');

      document.getElementById('back').addEventListener('click', () => navigateBack());

      let ctx;
      try {
        await loadSdk();
        ctx = await fundament.init;
      } catch (err) {
        intro.textContent = `Failed to load the plugin SDK: ${err?.message ?? err}`;
        ctx = null;
      }

      if (ctx) {
        intro.textContent = 'Declare an FSCInstallation to run an OpenFSC peer in a namespace.';
        renderNamespaceControl(ctx.namespaces);
        form.hidden = false;
      }

      // The namespace dropdown is fed by the host (project namespaces). When the
      // host has none (e.g. organization-level view), fall back to a text input.
      function renderNamespaceControl(namespaces) {
        const field = document.getElementById('namespace-field');
        const label = '<label class="plugin-label" for="namespace">Namespace</label>';
        if (Array.isArray(namespaces) && namespaces.length > 0) {
          field.innerHTML =
            label +
            `<select class="plugin-select" id="namespace" name="namespace" required>` +
            namespaces.map((n) => `<option value="${escapeHtml(n)}">${escapeHtml(n)}</option>`).join('') +
            `</select>`;
        } else {
          field.innerHTML =
            label +
            `<input class="plugin-input" id="namespace" name="namespace" required
               pattern="[a-z0-9]([a-z0-9\\-]*[a-z0-9])?" placeholder="my-namespace" />`;
        }
      }

      function applyMode() {
        const isExternal = form.elements['mode'].value === 'External';
        externalFieldset.hidden = !isExternal;
        // directory.external and certificate are required in External, forbidden in Self.
        form.querySelectorAll('[data-external-required]').forEach((el) => {
          el.required = isExternal;
        });
        // Per-gateway certificate overrides are only valid in External mode.
        document.querySelectorAll('.gw-cert-field').forEach((el) => {
          el.hidden = !isExternal;
        });
      }

      function addGateway(kind) {
        const row = document.createElement('div');
        row.className = 'plugin-row gateway-row';
        const selfAddress =
          kind === 'inway'
            ? `<div class="plugin-field" style="flex: 1 1 16rem">
                 <label class="plugin-label">Self address</label>
                 <input class="plugin-input gw-self" pattern="https://.+" placeholder="https://inway.example.com" />
               </div>`
            : '';
        row.innerHTML = `
          <div class="plugin-field" style="flex: 1 1 10rem">
            <label class="plugin-label">Name</label>
            <input class="plugin-input gw-name" required maxlength="30"
              pattern="[a-z0-9]([a-z0-9\\-]*[a-z0-9])?" placeholder="default" />
          </div>
          ${selfAddress}
          <div class="plugin-field gw-cert-field" style="flex: 1 1 14rem">
            <label class="plugin-label">Certificate secret</label>
            <input class="plugin-input gw-cert" placeholder="existing tls Secret name" />
          </div>
          <button type="button" class="plugin-button-secondary gw-remove">Remove</button>`;
        row.querySelector('.gw-remove').addEventListener('click', () => row.remove());
        document.getElementById(`${kind}s`).appendChild(row);
        applyMode();
      }

      function gatherGateways(kind, isExternal) {
        return [...document.querySelectorAll(`#${kind}s .gateway-row`)]
          .map((row) => {
            const name = row.querySelector('.gw-name').value.trim();
            if (!name) return null;
            const gw = { name };
            const self = row.querySelector('.gw-self');
            if (self && self.value.trim()) gw.selfAddress = self.value.trim();
            const cert = row.querySelector('.gw-cert');
            if (isExternal && cert && cert.value.trim()) {
              gw.certificate = { existingSecret: cert.value.trim() };
            }
            return gw;
          })
          .filter(Boolean);
      }

      function trimmed(id) {
        return (document.getElementById(id)?.value ?? '').trim();
      }

      function buildBody(namespace) {
        const mode = form.elements['mode'].value;
        const isExternal = mode === 'External';

        const spec = {
          groupID: trimmed('groupID'),
          peerID: trimmed('peerID'),
          directory: { mode },
          postgres: { storageClass: trimmed('pg-storageClass') },
        };

        const instances = trimmed('pg-instances');
        if (instances) spec.postgres.instances = Number(instances);
        const image = trimmed('pg-image');
        if (image) spec.postgres.image = image;
        const storageSize = trimmed('pg-storageSize');
        if (storageSize) spec.postgres.storageSize = storageSize;

        if (isExternal) {
          spec.directory.external = {
            address: trimmed('ext-address'),
            peerID: trimmed('ext-peerID'),
            trustAnchor: { name: trimmed('ext-ta-name'), key: trimmed('ext-ta-key') || 'ca.crt' },
          };
          const cert = trimmed('certificate');
          if (cert) spec.certificate = { existingSecret: cert };
        }

        const managerAddress = trimmed('managerAddress');
        if (managerAddress) spec.managerAddress = managerAddress;
        const controllerURL = trimmed('controllerURL');
        if (controllerURL) spec.controllerURL = controllerURL;

        const grants = [...form.querySelectorAll('input[name="autoSignGrants"]:checked')].map(
          (c) => c.value,
        );
        if (grants.length) spec.autoSignGrants = grants;

        const inways = gatherGateways('inway', isExternal);
        if (inways.length) spec.inways = inways;
        const outways = gatherGateways('outway', isExternal);
        if (outways.length) spec.outways = outways;

        return {
          apiVersion: 'openfsc.fundament.io/v1',
          kind: 'FSCInstallation',
          metadata: { name: trimmed('name'), namespace },
          spec,
        };
      }

      document.getElementById('mode').addEventListener('change', applyMode);
      document.getElementById('add-inway').addEventListener('click', () => addGateway('inway'));
      document.getElementById('add-outway').addEventListener('click', () => addGateway('outway'));
      applyMode();

      form.addEventListener('submit', async (e) => {
        e.preventDefault();
        errorBox.hidden = true;
        if (!form.reportValidity()) return;

        const submit = document.getElementById('submit');
        submit.disabled = true;
        try {
          const namespace = form.elements['namespace'].value.trim();
          const body = buildBody(namespace);
          const created = await fundament.k8s.create(
            { group: 'openfsc.fundament.io', version: 'v1', resource: 'fscinstallations', namespace },
            body,
          );
          navigateToDetail(created?.metadata?.name ?? body.metadata.name, namespace);
        } catch (err) {
          errorBox.textContent = `Failed to create: ${err?.message ?? err}`;
          errorBox.hidden = false;
          submit.disabled = false;
        }
      });
