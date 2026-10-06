import { escapeHtml, humanizeBytes } from './_shared.js';

// Which disks a pool may choose from. A disk this pool already uses reports
// available=false, since Ceph consumed it, so filtering on availability alone
// would empty the edit form the moment the pool went live. Disks claimed by
// another pool are never offered, mirroring ClaimOwner.
//
// poolName is null from the create form, where no pool owns anything yet.
export function selectableDisks(items, poolName) {
  return (items ?? []).filter((item) => {
    const s = item.status ?? {};
    if (s.claimedBy) return s.claimedBy === poolName;
    return Boolean(s.available);
  });
}

// One form section per node, so the operator can see the failure-domain spread,
// inside a #disk-picker group that carries the "select a disk" error.
export function renderDiskPicker(disks, selectedNames = []) {
  const selected = new Set(selectedNames);
  const byNode = new Map();
  for (const item of disks) {
    const node = item.status?.node ?? '(unknown node)';
    if (!byNode.has(node)) byNode.set(node, []);
    byNode.get(node).push(item);
  }

  const sections = [...byNode.entries()]
    .map(([node, nodeDisks]) => {
      const boxes = nodeDisks
        .map((disk) => {
          const s = disk.status ?? {};
          const name = disk.metadata?.name ?? '';
          // A filesystem the node named is really on the device, so say so here
          // rather than only on the detail page — this is the screen where the
          // disk gets taken. Marked, not filtered: a disk carrying BlueStore
          // from a dead cluster is exactly the one an operator needs to reuse,
          // and hiding it would leave no console path to reclaim it.
          const carries = s.filesystem ? `, contains ${s.filesystem}` : '';
          const label = `${s.path ?? name} (${humanizeBytes(s.sizeBytes ?? 0)}${carries})`;
          const checked = selected.has(name) ? ' checked' : '';
          return `
            <nldd-checkbox-field name="disk" value="${escapeHtml(name)}"
                                 label="${escapeHtml(label)}"${checked}></nldd-checkbox-field>`;
        })
        .join('');
      return `
        <nldd-form-section text="${escapeHtml(node)}">
          ${boxes}
        </nldd-form-section>`;
    })
    .join('');

  return `
    <div id="disk-picker" tabindex="-1">${sections}</div>
    <nldd-validation-list for="disk-picker">
      <nldd-validation-item id="disk-picker-error"></nldd-validation-item>
    </nldd-validation-list>`;
}

export function readSelectedDisks(formEl) {
  return Array.from(formEl.querySelectorAll('nldd-checkbox-field[name="disk"]'))
    .filter((cb) => cb.checked)
    .map((cb) => cb.value);
}
