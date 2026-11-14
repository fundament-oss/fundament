import {
  bootPage,
  errorBox,
  hintHtml,
  humanizeQuantity,
  loadingHtml,
  renderKeyValueList,
  sectionHtml,
} from './_shared.js';

const content = document.getElementById('content');
content.innerHTML = loadingHtml();

const ctx = await bootPage();

if (!ctx.resource?.name) {
  content.innerHTML = errorBox('no disk selected', 'Cannot show the disk');
} else {
  try {
    const item = await fundament.k8s.get({
      group: 'ceph.fundament.io',
      version: 'v1alpha1',
      resource: 'disks',
      name: ctx.resource.name,
    });

    const s = item.status ?? {};

    // Both paths: the kernel one matches lsblk, the stable one is what the pool
    // binds to and what survives a reboot.
    const device = [
      ['Path (kernel)', s.path ?? '—'],
      ['Path (stable)', s.stablePath || 'none reported; tracked by its kernel path'],
      ['Node', s.nodeName ?? '—'],
      ['Size', humanizeQuantity(s.size)],
      ['Type', s.type ?? '—'],
      ['Rotational', s.rotational ? 'yes' : 'no'],
      ['Model', s.model || '—'],
      ['Serial', s.serial || '—'],
      ['WWN', s.wwn || '—'],
    ];

    // Text, not a link: plugin:navigate resolves within the current kind, so a
    // hop to a DiskPool would route to disks/<poolname>.
    //
    // "Reported empty" rather than "Available": the flag is a node probe that
    // has been observed stale for hours while the device ran an OSD, so it is
    // reported as the observation it is and never as permission to take the disk.
    const allocation = [
      ['Claimed by', s.claimedBy || 'not claimed by any DiskPool'],
      ['Filesystem found', s.filesystem || 'none reported by the last probe'],
      ['Reported empty', s.available ? 'yes (the last probe found nothing on it)' : 'no'],
    ];

    content.innerHTML = `
      ${sectionHtml('Device', renderKeyValueList(device, 'Device'))}
      <nldd-spacer size="24"></nldd-spacer>
      ${sectionHtml(
        'Allocation',
        `${renderKeyValueList(allocation, 'Allocation')}
        <nldd-spacer size="12"></nldd-spacer>
        ${hintHtml(`“Claimed by” is set by Fundament: the DiskPool this disk belongs to.
          “Filesystem found” and “Reported empty” come from the node's last probe, which can
          lag. Confirm on the node before repurposing or removing a disk.`)}`,
      )}
    `;
  } catch (err) {
    content.innerHTML = errorBox(err);
  }
}
