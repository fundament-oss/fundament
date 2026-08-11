import { loadSdk, escapeHtml, humanizeBytes, renderDefList } from './_shared.js';

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

    const pairs = [
      ['Phase', status.phase ?? 'Unknown'],
      ['Storage Class', status.storageClassName ?? '—'],
      ['Replicas', String(status.replicas ?? '—')],
      ['Failure Domain', status.failureDomain ?? '—'],
      // Both counts describe the pool's selection, not what Ceph currently has
      // running — the labels say so rather than implying live cluster state.
      ['Selected disks', String(status.selectedDiskCount ?? '—')],
      ['Raw capacity (before replication)', humanizeBytes(status.rawCapacityBytes ?? 0)],
    ];
    if (status.replicas > 0 && status.rawCapacityBytes > 0) {
      pairs.push([
        'Usable capacity (approx.)',
        humanizeBytes(Math.floor(status.rawCapacityBytes / status.replicas)),
      ]);
    }
    if (status.message) pairs.push(['Message', status.message]);

    const disks =
      spec.disks && spec.disks.length > 0
        ? `<ul>${spec.disks.map((d) => `<li>${escapeHtml(d)}</li>`).join('')}</ul>`
        : '<p class="plugin-text">No disks selected.</p>';

    content.innerHTML = `
      <h2 class="plugin-heading">Status</h2>
      ${renderDefList(pairs)}
      <h2 class="plugin-heading">Selected Disks</h2>
      ${disks}
    `;
  } catch (err) {
    content.innerHTML = `<div class="plugin-error">${escapeHtml(`Failed to load: ${err?.message ?? err}`)}</div>`;
  }
}
