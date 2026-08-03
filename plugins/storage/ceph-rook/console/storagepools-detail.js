import { loadSdk, escapeHtml, humanizeBytes, emptyRow, errorRow } from './_shared.js';

await loadSdk();
const ctx = await fundament.init;
const content = document.getElementById('content');
const heading = document.getElementById('heading');

if (!ctx.resource?.name) {
  content.textContent = 'No storage pool selected.';
} else {
  try {
    const item = await fundament.k8s.get({
      group: 'storage.fundament.io',
      version: 'v1alpha1',
      resource: 'storagepools',
      name: ctx.resource.name,
    });

    heading.textContent = `Storage Pool · ${item.metadata?.name ?? ctx.resource.name}`;

    const status = item.status ?? {};
    const spec = item.spec ?? {};

    // Render status information.
    const statusHtml = `
      <h2 class="plugin-heading">Status</h2>
      <div style="margin-bottom: 1.5rem;">
        <div style="margin-bottom: 0.75rem;">
          <strong>Phase:</strong> <span class="plugin-badge" data-phase="${escapeHtml(status.phase?.toLowerCase() ?? 'unknown')}">${escapeHtml(status.phase ?? 'Unknown')}</span>
        </div>
        <div style="margin-bottom: 0.75rem;">
          <strong>Storage Class:</strong> ${escapeHtml(status.storageClassName ?? '—')}
        </div>
        <div style="margin-bottom: 0.75rem;">
          <strong>Replicas:</strong> ${escapeHtml(String(status.replicas ?? '—'))}
        </div>
        <div style="margin-bottom: 0.75rem;">
          <strong>Failure Domain:</strong> ${escapeHtml(status.failureDomain ?? '—')}
        </div>
        <div style="margin-bottom: 0.75rem;">
          <strong>OSD Count:</strong> ${escapeHtml(String(status.osdCount ?? '—'))}
        </div>
        <div style="margin-bottom: 0.75rem;">
          <strong>Capacity:</strong> ${escapeHtml(humanizeBytes(status.capacityBytes ?? 0))}
        </div>
        ${status.message ? `<div style="margin-bottom: 0.75rem;"><strong>Message:</strong> ${escapeHtml(status.message)}</div>` : ''}
      </div>
    `;

    // Render selected disks.
    const disksHtml = `
      <h2 class="plugin-heading">Selected Disks</h2>
      <div style="margin-bottom: 1.5rem;">
        ${
          spec.disks && spec.disks.length > 0
            ? `<ul style="list-style: disc; margin-left: 1.5rem;">
                ${spec.disks.map((disk) => `<li>${escapeHtml(disk)}</li>`).join('')}
              </ul>`
            : '<p class="plugin-text">No disks selected.</p>'
        }
      </div>
    `;

    content.innerHTML = statusHtml + disksHtml;
  } catch (err) {
    content.innerHTML = `<div class="plugin-text">${escapeHtml(`Failed to load: ${err?.message ?? err}`)}</div>`;
  }
}
