import { loadSdk, escapeHtml, humanizeBytes, renderDefList } from './_shared.js';

await loadSdk();
const ctx = await fundament.init;

const content = document.getElementById('content');
const heading = document.getElementById('heading');

if (!ctx.resource?.name) {
  content.textContent = 'No disk selected.';
} else {
  try {
    const item = await fundament.k8s.get({
      group: 'storage.fundament.io',
      version: 'v1alpha1',
      resource: 'disks',
      name: ctx.resource.name,
    });

    heading.textContent = `Disk · ${item.metadata?.name ?? ctx.resource.name}`;
    const s = item.status ?? {};

    const device = [
      ['Path', s.path ?? '—'],
      ['Node', s.node ?? '—'],
      ['Size', humanizeBytes(s.sizeBytes ?? 0)],
      ['Type', s.type ?? '—'],
      ['Rotational', s.rotational ? 'yes' : 'no'],
      ['Model', s.model || '—'],
      ['Serial', s.serial || '—'],
    ];

    // claimedBy is text, not a link: plugin:navigate resolves relative to the
    // current resource kind, so a hop to a StoragePool would route to
    // disks/<poolname>. Cross-kind navigation is not expressible today.
    const allocation = [
      ['Available', s.available ? 'yes' : 'no'],
      ['Claimed by', s.claimedBy || '—'],
    ];

    content.innerHTML = `
      <h2 class="plugin-heading">Device</h2>
      ${renderDefList(device)}
      <h2 class="plugin-heading">Allocation</h2>
      ${renderDefList(allocation)}
      <p class="plugin-text">
        A disk stops reporting as available once Ceph consumes it, so “Claimed by” is what
        tells you whether that was this plugin.
      </p>
    `;
  } catch (err) {
    content.innerHTML = `<div class="plugin-error">${escapeHtml(
      `Failed to load: ${err?.message ?? err}`,
    )}</div>`;
  }
}
