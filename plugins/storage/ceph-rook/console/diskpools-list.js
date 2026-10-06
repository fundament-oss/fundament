import {
  loadSdk,
  openSheet,
  errorBox,
  ensureNldd,
  escapeHtml,
  emptyRow,
  errorRow,
  wireRowLinks,
  navigateToDetail,
  humanizeQuantity,
  resourceNameError,
  wireSubmit,
} from './_shared.js';
import { selectableDisks, renderDiskPicker, readSelectedDisks } from './disk-picker.js';

await loadSdk();
await fundament.init;
// Warm the sheet bundle so the first Create click opens instantly.
ensureNldd().catch(() => {});

const tbody = document.getElementById('rows');

document.getElementById('create-btn').addEventListener('click', () => openCreateSheet());

// The create form lives in a sheet over the list. Disks are fetched at open
// time, so the picker reflects the latest probe.
async function openCreateSheet() {
  const sheet = await openSheet({ label: 'Create Disk Pool' });
  if (!sheet) return;
  const { body, close } = sheet;
  body.insertAdjacentHTML('beforeend', '<p class="plugin-text">Loading disks…</p>');

  let availableDisks;
  try {
    const { items } = await fundament.k8s.list({
      group: 'ceph.fundament.io',
      version: 'v1alpha1',
      resource: 'disks',
    });
    // Only unclaimed, available disks; null because no pool exists yet.
    availableDisks = selectableDisks(items, null);
  } catch (err) {
    body.lastElementChild.outerHTML = errorBox(err, 'Failed to load disks');
    return;
  }

  if (availableDisks.length === 0) {
    body.lastElementChild.outerHTML = `
      <p class="plugin-text">
        No disks to offer. A disk appears here when it is discovered, unclaimed, and empty
        in the node's last probe. The probe can lag; <code>kubectl get disks</code> shows
        what each disk reports.
      </p>
    `;
    return;
  }

  body.lastElementChild.outerHTML = `
    <p class="plugin-text">
      <strong>Recommendation:</strong> create a single DiskPool per cluster. All pools feed
      one shared Ceph cluster and data is placed across every disk in it, so a second pool
      only contributes more disks.
    </p>

    <form class="plugin-form" novalidate>
      <div class="plugin-error" data-role="error" hidden></div>

      <div class="plugin-field">
        <label class="plugin-label" for="pool-name">Name</label>
        <input id="pool-name" name="name" type="text" class="plugin-input"
               placeholder="default" required
               pattern="[a-z0-9]([a-z0-9\\-]*[a-z0-9])?" maxlength="63" />
        <span class="plugin-hint">Lowercase letters, digits and dashes.</span>
      </div>

      <div class="plugin-field">
        <span class="plugin-label">Disks</span>
        ${renderDiskPicker(availableDisks)}
        <span class="plugin-hint">
          Each selected disk joins the shared Ceph cluster, which runs one storage daemon
          (OSD) per disk. Disks spread over two or more nodes let volumes survive a node
          failure. Fundament confirms only that a disk is unclaimed. A disk marked as
          carrying a filesystem holds data, and one marked with nothing may still hold data
          the last probe missed.
        </span>
      </div>

      <div class="plugin-actions">
        <button type="submit" class="plugin-button" data-role="submit">Create DiskPool</button>
        <button type="button" class="plugin-button-secondary" data-role="cancel">Cancel</button>
      </div>
    </form>
  `;

  const form = body.querySelector('form');
  const nameInput = form.querySelector('[name="name"]');
  body.querySelector('[data-role="cancel"]').addEventListener('click', () => close());

  wireSubmit(form, {
    button: body.querySelector('[data-role="submit"]'),
    errorBox: body.querySelector('[data-role="error"]'),
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
      close();
      navigateToDetail(name);
    },
  });
}

try {
  const { items } = await fundament.k8s.list({
    group: 'ceph.fundament.io',
    version: 'v1alpha1',
    resource: 'diskpools',
  });

  if (!items || items.length === 0) {
    tbody.innerHTML = emptyRow(5, 'No disk pools.');
  } else {
    tbody.innerHTML = items
      .map((item) => {
        const status = item.status ?? {};
        const name = item.metadata?.name ?? '';
        return `
          <tr data-name="${escapeHtml(name)}">
            <td><a href="#" class="row-link">${escapeHtml(name)}</a></td>
            <td>${escapeHtml(status.phase ?? 'Unknown')}</td>
            <td>${escapeHtml(String(status.selectedDiskCount ?? '—'))}</td>
            <td>${escapeHtml(humanizeQuantity(status.rawCapacity))}</td>
            <td>${escapeHtml(status.message ?? '')}</td>
          </tr>`;
      })
      .join('');
    wireRowLinks(tbody);
  }
} catch (err) {
  tbody.innerHTML = errorRow(5, err);
}
