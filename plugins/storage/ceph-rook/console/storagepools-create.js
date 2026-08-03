import { loadSdk, escapeHtml, humanizeBytes } from './_shared.js';

await loadSdk();
await fundament.init;

const content = document.getElementById('content');

// --- Load available disks ---

let availableDisks = [];

try {
  const { items } = await fundament.k8s.list({
    group: 'storage.fundament.io',
    version: 'v1alpha1',
    resource: 'disks',
  });

  // Filter to disks that are available and unclaimed.
  availableDisks = (items ?? []).filter(
    (item) => item.status?.available && !item.status?.claimedBy,
  );
} catch (err) {
  content.innerHTML = `<p class="plugin-text">${escapeHtml(`Failed to load disks: ${err?.message ?? err}`)}</p>`;
  // Do not render the form — operator cannot pick disks without the list.
  // fall through: availableDisks stays empty and the no-disks branch handles it below.
}

// --- Render form ---

if (availableDisks.length === 0 && content.innerHTML.includes('Failed')) {
  // Error already rendered above — leave content as-is.
} else if (availableDisks.length === 0) {
  // No unclaimed disks.
  content.innerHTML = `
    <p class="plugin-text">
      No unclaimed, available disks found in the cluster. Disks must be discovered and
      available (not already claimed by another StoragePool) before a StoragePool can be created.
    </p>
    <p style="margin-top: 0.75rem;">
      <a href="../storagepools-list.html" class="plugin-button"
         style="display: inline-block; padding: 0.5rem 1rem; background-color: #6b7280; color: white; border-radius: 0.375rem; text-decoration: none;">
        Back to Storage Pools
      </a>
    </p>
  `;
} else {
  // Group disks by node.
  const byNode = new Map();
  for (const item of availableDisks) {
    const node = item.status?.node ?? '(unknown node)';
    if (!byNode.has(node)) byNode.set(node, []);
    byNode.get(node).push(item);
  }

  // Build disk-picker HTML grouped by node.
  const diskPickerRows = [];
  for (const [node, disks] of byNode) {
    diskPickerRows.push(`
      <div style="margin-bottom: 0.5rem; font-weight: 600; color: #374151;">
        ${escapeHtml(node)}
      </div>`);
    for (const disk of disks) {
      const s = disk.status ?? {};
      const diskName = disk.metadata?.name ?? '';
      const label = `${escapeHtml(s.path ?? diskName)} — ${escapeHtml(humanizeBytes(s.sizeBytes ?? 0))}`;
      diskPickerRows.push(`
      <div style="margin-left: 1rem; margin-bottom: 0.375rem;">
        <label style="display: flex; align-items: center; gap: 0.5rem; cursor: pointer;">
          <input type="checkbox" name="disk" value="${escapeHtml(diskName)}" style="width: 1rem; height: 1rem;" />
          <span class="plugin-text" style="margin: 0;">${label}</span>
        </label>
      </div>`);
    }
    diskPickerRows.push('<div style="margin-bottom: 0.75rem;"></div>');
  }

  content.innerHTML = `
    <p class="plugin-text" style="margin-bottom: 1.25rem;">
      <strong>Recommendation:</strong> Create a single StoragePool per cluster for most setups.
      Multiple StoragePools are only needed for advanced tiered-storage configurations.
    </p>

    <form id="create-form" novalidate>
      <div style="margin-bottom: 1.25rem;">
        <label class="plugin-text" style="display: block; font-weight: 600; margin-bottom: 0.375rem;" for="pool-name">
          Name <span style="color: #ef4444;">*</span>
        </label>
        <input
          id="pool-name"
          name="name"
          type="text"
          class="plugin-input"
          placeholder="e.g. default"
          required
          style="width: 100%; max-width: 24rem; padding: 0.5rem 0.75rem; border: 1px solid #d1d5db; border-radius: 0.375rem; font-size: 0.875rem;"
        />
      </div>

      <div style="margin-bottom: 1.25rem;">
        <div style="font-weight: 600; margin-bottom: 0.5rem; color: #111827;" class="plugin-text">
          Disks <span style="color: #ef4444;">*</span>
        </div>
        <div id="disk-picker" style="border: 1px solid #e5e7eb; border-radius: 0.375rem; padding: 0.75rem; max-height: 20rem; overflow-y: auto;">
          ${diskPickerRows.join('')}
        </div>
      </div>

      <div style="margin-bottom: 1.5rem;">
        <label class="plugin-text" style="display: block; font-weight: 600; margin-bottom: 0.375rem;" for="replication">
          Replication
        </label>
        <select
          id="replication"
          name="replication"
          style="padding: 0.5rem 0.75rem; border: 1px solid #d1d5db; border-radius: 0.375rem; font-size: 0.875rem; background-color: white;"
        >
          <option value="auto" selected>auto (recommended)</option>
          <option value="1">1 — no replication</option>
          <option value="2">2 — two replicas</option>
          <option value="3">3 — three replicas</option>
        </select>
      </div>

      <div
        id="error-box"
        role="alert"
        hidden
        style="margin-bottom: 1rem; padding: 0.75rem 1rem; background-color: #fef2f2; border: 1px solid #fca5a5; border-radius: 0.375rem; color: #991b1b; font-size: 0.875rem;"
      ></div>

      <div style="display: flex; gap: 0.75rem; align-items: center;">
        <button
          id="submit-btn"
          type="submit"
          class="plugin-button"
          style="padding: 0.5rem 1.25rem; background-color: #3b82f6; color: white; border: none; border-radius: 0.375rem; font-size: 0.875rem; cursor: pointer;"
        >
          Create StoragePool
        </button>
        <a
          href="../storagepools-list.html"
          class="plugin-button"
          style="padding: 0.5rem 1rem; background-color: #6b7280; color: white; border-radius: 0.375rem; text-decoration: none; font-size: 0.875rem;"
        >
          Cancel
        </a>
      </div>
    </form>
  `;

  // --- Form submission logic ---

  const form = document.getElementById('create-form');
  const errorBox = document.getElementById('error-box');
  const submitBtn = document.getElementById('submit-btn');

  function showError(message) {
    errorBox.textContent = message;
    errorBox.hidden = false;
  }

  function clearError() {
    errorBox.textContent = '';
    errorBox.hidden = true;
  }

  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    clearError();

    // Validate: name must be non-empty.
    const nameInput = form.querySelector('[name="name"]');
    const name = nameInput.value.trim();
    if (!name) {
      showError('Please enter a name for the StoragePool.');
      nameInput.focus();
      return;
    }

    // Validate: at least one disk must be selected.
    const checkedDisks = Array.from(form.querySelectorAll('[name="disk"]:checked')).map(
      (cb) => cb.value,
    );
    if (checkedDisks.length === 0) {
      showError('Please select at least one disk.');
      return;
    }

    const replication = form.querySelector('[name="replication"]').value;

    // Build the StoragePool CR body.
    const body = {
      apiVersion: 'storage.fundament.io/v1alpha1',
      kind: 'StoragePool',
      metadata: { name },
      spec: {
        disks: checkedDisks,
        replication,
      },
    };

    // Disable submit while in flight.
    submitBtn.disabled = true;
    submitBtn.textContent = 'Creating…';

    try {
      await fundament.k8s.create(
        { group: 'storage.fundament.io', version: 'v1alpha1', resource: 'storagepools' },
        body,
      );
      // Navigate to the detail view for the newly created pool.
      window.parent.postMessage({ type: 'plugin:navigate', name }, '*');
    } catch (err) {
      showError(`Failed to create StoragePool: ${err?.message ?? err}`);
      submitBtn.disabled = false;
      submitBtn.textContent = 'Create StoragePool';
    }
  });
}
