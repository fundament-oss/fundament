import {
  addInfoButton,
  bootPage,
  errorBox,
  humanizeQuantity,
  loadingHtml,
  renderKeyValueList,
  sectionHtml,
} from './_shared.js';

const content = document.getElementById('content');
content.innerHTML = loadingHtml();

const ctx = await bootPage();

addInfoButton(document.getElementById('page-actions'), 'About disks', [
  'A disk pool tracks a disk by its stable path, which stays the same after a reboot. A disk without one is tracked by its device path.',
  'Disk pool is set by Fundament when a disk pool takes the disk.',
  'Filesystem found and Reported empty come from the node\'s last check, which can be hours old. Check on the node itself before you reuse or remove a disk.',
]);

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

    // Both paths: the device path matches lsblk, the stable one is what the
    // pool binds to and what survives a reboot.
    const device = [
      ['Device path', s.path ?? '—'],
      ['Stable path', s.stablePath || 'None'],
      ['Node', s.nodeName ?? '—'],
      ['Size', humanizeQuantity(s.size)],
      ['Type', s.type ?? '—'],
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
      ['Disk pool', s.claimedBy || 'None'],
      ['Filesystem found', s.filesystem || 'None'],
      ['Reported empty', s.available ? 'Yes' : 'No'],
    ];

    content.innerHTML = `
      ${sectionHtml('Device', renderKeyValueList(device, 'Device'))}
      <nldd-spacer size="24"></nldd-spacer>
      ${sectionHtml('Usage', renderKeyValueList(allocation, 'Usage'))}
    `;
  } catch (err) {
    content.innerHTML = errorBox(err);
  }
}
