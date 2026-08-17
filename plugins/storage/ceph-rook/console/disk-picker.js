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

// One fieldset per node, so the operator can see the failure-domain spread.
export function renderDiskPicker(disks, selectedNames = []) {
  const selected = new Set(selectedNames);
  const byNode = new Map();
  for (const item of disks) {
    const node = item.status?.node ?? '(unknown node)';
    if (!byNode.has(node)) byNode.set(node, []);
    byNode.get(node).push(item);
  }

  return [...byNode.entries()]
    .map(([node, nodeDisks]) => {
      const boxes = nodeDisks
        .map((disk) => {
          const s = disk.status ?? {};
          const name = disk.metadata?.name ?? '';
          const label = `${s.path ?? name} — ${humanizeBytes(s.sizeBytes ?? 0)}`;
          const checked = selected.has(name) ? ' checked' : '';
          return `
            <label class="plugin-checkbox">
              <input type="checkbox" name="disk" value="${escapeHtml(name)}"${checked} />
              <span>${escapeHtml(label)}</span>
            </label>`;
        })
        .join('');
      return `
        <fieldset class="plugin-fieldset">
          <legend class="plugin-legend">${escapeHtml(node)}</legend>
          ${boxes}
        </fieldset>`;
    })
    .join('');
}

export function readSelectedDisks(formEl) {
  return Array.from(formEl.querySelectorAll('[name="disk"]:checked')).map((cb) => cb.value);
}
