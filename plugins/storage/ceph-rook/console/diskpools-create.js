import {
  loadSdk,
  escapeHtml,
  navigateToDetail,
  navigateBack,
  resourceNameError,
  wireSubmit,
} from './_shared.js';
import { selectableDisks, renderDiskPicker, readSelectedDisks } from './disk-picker.js';

await loadSdk();
await fundament.init;

const content = document.getElementById('content');

let availableDisks = [];
let loadError = null;

try {
  const { items } = await fundament.k8s.list({
    group: 'ceph.fundament.io',
    version: 'v1alpha1',
    resource: 'disks',
  });

  // Only unclaimed, available disks. The DiskInventory reconciler re-runs on
  // DiskPool changes, so a disk another pool just took drops out. null
  // because no pool exists yet at create time.
  availableDisks = selectableDisks(items, null);
} catch (err) {
  loadError = err;
}

if (loadError) {
  content.innerHTML = `<div class="plugin-error">${escapeHtml(
    `Failed to load disks: ${loadError?.message ?? loadError}`,
  )}</div>`;
} else if (availableDisks.length === 0) {
  content.innerHTML = `
    <p class="plugin-text">
      No disks to offer. A disk appears here once it has been discovered, is not claimed by
      another DiskPool, and the node's last probe found nothing on it. That probe can lag,
      so a disk missing from this list is not necessarily in use — check
      <code>kubectl get disks</code> for what each one reports.
    </p>
    <div class="plugin-actions">
      <button type="button" class="plugin-button-secondary" id="back-btn">Back to Disk Pools</button>
    </div>
  `;
  document.getElementById('back-btn').addEventListener('click', () => navigateBack());
} else {
  const diskPicker = renderDiskPicker(availableDisks);

  content.innerHTML = `
    <p class="plugin-text">
      <strong>Recommendation:</strong> create a single DiskPool per cluster. All pools feed
      one shared Ceph cluster and data is placed across every disk in it, so a second pool
      only contributes more disks — not isolated or tiered storage.
    </p>

    <form id="create-form" class="plugin-form" novalidate>
      <div class="plugin-error" id="error-box" hidden></div>

      <div class="plugin-field">
        <label class="plugin-label" for="pool-name">Name</label>
        <input id="pool-name" name="name" type="text" class="plugin-input"
               placeholder="default" required
               pattern="[a-z0-9]([a-z0-9\\-]*[a-z0-9])?" maxlength="63" />
        <span class="plugin-hint">Lowercase letters, digits and dashes.</span>
      </div>

      <div class="plugin-field">
        <span class="plugin-label">Disks</span>
        ${diskPicker}
        <span class="plugin-hint">
          These disks become OSDs in the shared Ceph cluster. Disks spread over two or more
          nodes enable host-level failure domains. Fundament can confirm only that a disk is
          unclaimed, not that it is empty — a disk marked as carrying a filesystem holds data,
          and one marked with nothing may still hold data the last node probe missed.
        </span>
      </div>

      <div class="plugin-actions">
        <button id="submit-btn" type="submit" class="plugin-button">Create DiskPool</button>
        <button id="cancel-btn" type="button" class="plugin-button-secondary">Cancel</button>
      </div>
    </form>
  `;

  const form = document.getElementById('create-form');
  const nameInput = form.querySelector('[name="name"]');

  document.getElementById('cancel-btn').addEventListener('click', () => navigateBack());

  wireSubmit(form, {
    button: document.getElementById('submit-btn'),
    errorBox: document.getElementById('error-box'),
    busyLabel: 'Creating…',
    failPrefix: 'Failed to create DiskPool',
    validate: () => {
      const invalid = resourceNameError(nameInput.value.trim());
      if (invalid) {
        nameInput.focus();
        return invalid;
      }
      if (readSelectedDisks(form).length === 0) {
        return 'Please select at least one disk.';
      }
      return null;
    },
    action: async () => {
      const name = nameInput.value.trim();
      await fundament.k8s.create(
        { group: 'ceph.fundament.io', version: 'v1alpha1', resource: 'diskpools' },
        {
          apiVersion: 'ceph.fundament.io/v1alpha1',
          kind: 'DiskPool',
          metadata: { name },
          spec: { disks: readSelectedDisks(form) },
        },
      );
      navigateToDetail(name);
    },
  });
}
