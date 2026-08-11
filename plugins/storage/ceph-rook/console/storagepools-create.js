import { loadSdk, escapeHtml, humanizeBytes, navigateToDetail, navigateBack } from './_shared.js';

await loadSdk();
await fundament.init;

const content = document.getElementById('content');

// --- Load available disks ---

let availableDisks = [];
let loadError = null;

try {
  const { items } = await fundament.k8s.list({
    group: 'storage.fundament.io',
    version: 'v1alpha1',
    resource: 'disks',
  });

  // Only unclaimed, available disks can be pooled. claimedBy is maintained by
  // the DiskInventory reconciler, which re-runs when a StoragePool changes, so
  // a disk another pool just took disappears from this list.
  availableDisks = (items ?? []).filter(
    (item) => item.status?.available && !item.status?.claimedBy,
  );
} catch (err) {
  loadError = err;
}

// --- Render ---

if (loadError) {
  content.innerHTML = `<div class="plugin-error">${escapeHtml(
    `Failed to load disks: ${loadError?.message ?? loadError}`,
  )}</div>`;
} else if (availableDisks.length === 0) {
  content.innerHTML = `
    <p class="plugin-text">
      No unclaimed, available disks found in the cluster. Disks must be discovered and
      available (not already claimed by another StoragePool) before a StoragePool can be created.
    </p>
    <div class="plugin-actions">
      <button type="button" class="plugin-button-secondary" id="back-btn">Back to Storage Pools</button>
    </div>
  `;
  document.getElementById('back-btn').addEventListener('click', () => navigateBack());
} else {
  // Group disks by node so the operator can see the failure-domain spread.
  const byNode = new Map();
  for (const item of availableDisks) {
    const node = item.status?.node ?? '(unknown node)';
    if (!byNode.has(node)) byNode.set(node, []);
    byNode.get(node).push(item);
  }

  const diskPicker = [...byNode.entries()]
    .map(([node, disks]) => {
      const boxes = disks
        .map((disk) => {
          const s = disk.status ?? {};
          const diskName = disk.metadata?.name ?? '';
          const label = `${s.path ?? diskName} — ${humanizeBytes(s.sizeBytes ?? 0)}`;
          return `
            <label class="plugin-checkbox">
              <input type="checkbox" name="disk" value="${escapeHtml(diskName)}" />
              <span>${escapeHtml(label)}</span>
            </label>`;
        })
        .join('');
      return `
        <fieldset class="plugin-fieldset">
          <legend class="plugin-legend">${escapeHtml(node)}</legend>
          ${boxes}
        </fieldset>`;
    })
    .join('');

  content.innerHTML = `
    <p class="plugin-text">
      <strong>Recommendation:</strong> create a single StoragePool per cluster for most setups.
      Multiple StoragePools are only needed for advanced tiered-storage configurations.
    </p>

    <form id="create-form" class="plugin-form" novalidate>
      <div class="plugin-error" id="error-box" hidden></div>

      <div class="plugin-field">
        <label class="plugin-label" for="pool-name">Name</label>
        <input id="pool-name" name="name" type="text" class="plugin-input"
               placeholder="default" required
               pattern="[a-z0-9]([a-z0-9\\-]*[a-z0-9])?" maxlength="63" />
        <span class="plugin-hint">Lowercase letters, digits and dashes. Names the resulting StorageClass.</span>
      </div>

      <div class="plugin-field">
        <span class="plugin-label">Disks</span>
        ${diskPicker}
        <span class="plugin-hint">Disks spread over two or more nodes enable host-level failure domains.</span>
      </div>

      <div class="plugin-field">
        <label class="plugin-label" for="replication">Replication</label>
        <select id="replication" name="replication" class="plugin-select">
          <option value="auto" selected>auto (recommended)</option>
          <option value="1">1 — no replication</option>
          <option value="2">2 — two replicas</option>
          <option value="3">3 — three replicas</option>
        </select>
        <span class="plugin-hint">auto derives the replica count from the number of contributing nodes.</span>
      </div>

      <div class="plugin-actions">
        <button id="submit-btn" type="submit" class="plugin-button">Create StoragePool</button>
        <button id="cancel-btn" type="button" class="plugin-button-secondary">Cancel</button>
      </div>
    </form>
  `;

  const form = document.getElementById('create-form');
  const errorBox = document.getElementById('error-box');
  const submitBtn = document.getElementById('submit-btn');

  function showError(message) {
    errorBox.textContent = message;
    errorBox.hidden = false;
  }

  document.getElementById('cancel-btn').addEventListener('click', () => navigateBack());

  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    errorBox.hidden = true;

    const nameInput = form.querySelector('[name="name"]');
    const name = nameInput.value.trim();
    if (!name) {
      showError('Please enter a name for the StoragePool.');
      nameInput.focus();
      return;
    }
    if (!/^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/.test(name)) {
      showError('Use lowercase letters, digits and dashes only.');
      nameInput.focus();
      return;
    }

    const checkedDisks = Array.from(form.querySelectorAll('[name="disk"]:checked')).map(
      (cb) => cb.value,
    );
    if (checkedDisks.length === 0) {
      showError('Please select at least one disk.');
      return;
    }

    const replication = form.querySelector('[name="replication"]').value;

    submitBtn.disabled = true;
    submitBtn.textContent = 'Creating…';

    try {
      await fundament.k8s.create(
        { group: 'storage.fundament.io', version: 'v1alpha1', resource: 'storagepools' },
        {
          apiVersion: 'storage.fundament.io/v1alpha1',
          kind: 'StoragePool',
          metadata: { name },
          spec: { disks: checkedDisks, replication },
        },
      );
      // From a create view the host hops to the new resource's sibling detail route.
      navigateToDetail(name);
    } catch (err) {
      showError(`Failed to create StoragePool: ${err?.message ?? err}`);
      submitBtn.disabled = false;
      submitBtn.textContent = 'Create StoragePool';
    }
  });
}
