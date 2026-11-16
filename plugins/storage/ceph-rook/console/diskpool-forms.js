// The DiskPool create and edit forms, which open in a console sheet over
// the list and detail pages. sheet.js boots the SDK and the design system
// bundle first (bootPage).
import {
  errorBox,
  escapeHtml,
  navigateToDetail,
  resourceNameError,
  wireSubmit,
  formFieldHtml,
  formActionsHtml,
  errorBannerHtml,
  loadErrorBanner,
} from './_shared.js';
import { selectableDisks, renderDiskPicker, readSelectedDisks } from './disk-picker.js';

const RESOURCE = {
  group: 'ceph.fundament.io',
  version: 'v1alpha1',
  resource: 'diskpools',
};

const RESOURCE_DISKS = {
  group: 'ceph.fundament.io',
  version: 'v1alpha1',
  resource: 'disks',
};

// The create form, which the list opens in a console sheet (see sheet.js);
// also the routed create page, where closeSheet goes back to the list. Disks
// are fetched when it opens, so the picker reflects the latest probe.
export async function diskPoolCreatePage() {
  const body = document.getElementById('content');
  body.innerHTML =
    '<nldd-activity-indicator show-text text="Loading disks…"></nldd-activity-indicator>';

  let availableDisks;
  try {
    const { items } = await fundament.k8s.list(RESOURCE_DISKS);
    // Only unclaimed, available disks; null because no pool exists yet.
    availableDisks = selectableDisks(items, null);
  } catch (err) {
    body.replaceChildren(loadErrorBanner(`Failed to load disks: ${err?.message ?? err}`));
    return;
  }

  if (availableDisks.length === 0) {
    body.innerHTML = `
      <nldd-rich-text>
        <p>
          No disks to offer. A disk appears here when it is discovered, unclaimed, and empty
          in the node's last probe. The probe can lag; <code>kubectl get disks</code> shows
          what each disk reports.
        </p>
      </nldd-rich-text>
    `;
    return;
  }

  body.innerHTML = `
    <nldd-rich-text>
      <p>
        <strong>Recommendation:</strong> create a single DiskPool per cluster. All pools feed
        one shared Ceph cluster and data is placed across every disk in it, so a second pool
        only contributes more disks.
      </p>
    </nldd-rich-text>
    <nldd-spacer size="12"></nldd-spacer>

    <nldd-form>
      <form novalidate>
        ${errorBannerHtml('error-banner')}

        ${formFieldHtml(
          'Name',
          `<nldd-text-field id="pool-name" name="name" placeholder="default"
                            required maxlength="63" no-spellcheck></nldd-text-field>`,
          { errorId: 'pool-name-error', hint: 'Lowercase letters, digits and dashes.' },
        )}

        <nldd-form-section text="Disks">
          ${renderDiskPicker(availableDisks)}
          <nldd-text size="sm" color="secondary">
            Each selected disk joins the shared Ceph cluster, which runs one storage daemon
            (OSD) per disk. Disks spread over two or more nodes let volumes survive a node
            failure. Fundament confirms only that a disk is unclaimed. A disk marked as
            carrying a filesystem holds data, and one marked with nothing may still hold data
            the last probe missed.
          </nldd-text>
        </nldd-form-section>

        ${formActionsHtml({ submitId: 'submit-btn', submitText: 'Create disk pool', cancelId: 'cancel-btn' })}
      </form>
    </nldd-form>
  `;

  const form = body.querySelector('form');
  const nameInput = form.querySelector('[name="name"]');
  body.querySelector('#cancel-btn').addEventListener('click', () => fundament.closeSheet());

  wireSubmit(form, {
    button: body.querySelector('#submit-btn'),
    errorBanner: body.querySelector('#error-banner'),
    failPrefix: 'Failed to create disk pool',
    checks: [
      [nameInput, resourceNameError],
      [
        body.querySelector('#disk-picker'),
        () => (readSelectedDisks(form).length === 0 ? 'Please select at least one disk.' : null),
      ],
    ],
    action: async () => {
      const name = nameInput.value.trim();
      await fundament.k8s.create(
        RESOURCE,
        {
          apiVersion: 'ceph.fundament.io/v1alpha1',
          kind: 'DiskPool',
          metadata: { name },
          spec: { disks: readSelectedDisks(form) },
        },
      );
      // From a sheet the console closes it and shows the new pool.
      navigateToDetail(name);
    },
  });
}

// The edit form, which the detail page opens in a console sheet. Saving
// closes the sheet, and the detail page reads the pool again.
export async function diskPoolEditPage(ctx) {
  const body = document.getElementById('content');
  const name = ctx.resource?.name;
  if (!name) {
    body.innerHTML = errorBox('no disk pool given', 'Cannot edit');
    return;
  }

  body.innerHTML =
    '<nldd-activity-indicator show-text text="Loading disks…"></nldd-activity-indicator>';

  let item;
  let disks;
  try {
    const [pool, { items }] = await Promise.all([
      fundament.k8s.get({ ...RESOURCE, name }),
      fundament.k8s.list(RESOURCE_DISKS),
    ]);
    item = pool;
    disks = selectableDisks(items, name);
  } catch (err) {
    body.replaceChildren(loadErrorBanner(`Failed to load the disk pool: ${err?.message ?? err}`));
    return;
  }
  const current = item.spec?.disks ?? [];
  const currentNames = current.map((d) => d.name);

  // The picker can only offer disks it can see: one claimed by another pool, or
  // named in spec with no Disk CR behind it, never gets a checkbox. Save rebuilds
  // spec.disks wholesale, so anything the form did not render used to disappear
  // on a save the operator did not intend as a change — one click after the
  // detail page flagged that exact disk. Carry them through untouched, and name
  // them below rather than holding them silently.
  const rendered = new Set(disks.map((d) => d.metadata?.name).filter(Boolean));
  const preserved = current.filter((d) => !rendered.has(d.name));
  const preservedNote =
    preserved.length === 0
      ? ''
      : `<nldd-text size="sm" color="secondary">
           Kept as they are, because this form cannot show them: a disk is listed here only
           when it exists and is either free or already claimed by this pool:
           ${escapeHtml(preserved.map((d) => d.name).join(', '))}. Saving leaves them in the pool; use kubectl
           to remove one.
         </nldd-text>`;

  body.innerHTML = `
    <nldd-form>
      <form novalidate>
        ${errorBannerHtml('edit-error')}

        <nldd-form-section text="Disks">
          ${renderDiskPicker(disks, currentNames)}
          ${preservedNote}
          <nldd-text size="sm" color="secondary">
            Unchecking a disk removes it from the shared Ceph cluster's device list, but its
            storage daemon (OSD) keeps running until it is purged from Ceph manually, and
            data may rebalance in the meantime.
          </nldd-text>
        </nldd-form-section>

        ${formActionsHtml({ submitId: 'save-btn', submitText: 'Save', cancelId: 'cancel-btn' })}
      </form>
    </nldd-form>
  `;

  const form = body.querySelector('form');

  body.querySelector('#cancel-btn').addEventListener('click', () => fundament.closeSheet());

  // Disjoint by construction: preserved is exactly what the picker did not
  // render, so this cannot produce the duplicate name the CRD's listType=map
  // rejects.
  const selected = () => [...readSelectedDisks(form, current), ...preserved];

  wireSubmit(form, {
    button: body.querySelector('#save-btn'),
    errorBanner: body.querySelector('#edit-error'),
    failPrefix: 'Failed to save',
    checks: [
      [
        body.querySelector('#disk-picker'),
        () => (selected().length === 0 ? 'Select at least one disk.' : null),
      ],
    ],
    action: async () => {
      // Merge-patch of spec only: status is untouched and disks is replaced
      // wholesale, not merged element-wise.
      await fundament.k8s.patch(
        { ...RESOURCE, name },
        {
          spec: { disks: selected() },
        },
      );
      fundament.closeSheet({ changed: true });
    },
  });
}
