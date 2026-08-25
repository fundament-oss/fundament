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

    // Both paths: the kernel one matches lsblk, the stable one is what the pool
    // binds to and what survives a reboot.
    const device = [
      ['Path (kernel)', s.path ?? '—'],
      ['Path (stable)', s.stablePath || 'none reported — this device is tracked by its kernel path'],
      ['Node', s.node ?? '—'],
      ['Size', humanizeBytes(s.sizeBytes ?? 0)],
      ['Type', s.type ?? '—'],
      ['Rotational', s.rotational ? 'yes' : 'no'],
      ['Model', s.model || '—'],
      ['Serial', s.serial || '—'],
      ['WWN', s.wwn || '—'],
    ];

    // Text, not a link: plugin:navigate resolves within the current kind, so a
    // hop to a StoragePool would route to disks/<poolname>.
    //
    // "Reported empty" rather than "Available": the flag is a node probe that
    // has been observed stale for hours while the device ran an OSD, so it is
    // reported as the observation it is and never as permission to take the disk.
    const allocation = [
      ['Claimed by', s.claimedBy || 'not claimed by any StoragePool'],
      ['Filesystem found', s.filesystem || 'none reported by the last probe'],
      ['Reported empty', s.available ? 'yes — last node probe found nothing on it' : 'no'],
    ];

    content.innerHTML = `
      <h2 class="plugin-heading">Device</h2>
      ${renderDefList(device)}
      <h2 class="plugin-heading">Allocation</h2>
      ${renderDefList(allocation)}
      <p class="plugin-text">
        “Claimed by” is the only line Fundament controls, and it says whether this plugin
        handed the disk to a pool. The two below it are the node's last probe: a filesystem
        it names is really there, but the probe can lag, so “Reported empty: yes” is not a
        guarantee the disk is unused. Confirm on the node before repurposing or pulling it.
      </p>
    `;
  } catch (err) {
    content.innerHTML = `<div class="plugin-error">${escapeHtml(
      `Failed to load: ${err?.message ?? err}`,
    )}</div>`;
  }
}
