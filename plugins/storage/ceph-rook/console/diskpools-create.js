import {
  loadSdk,
  loadNlddDesignSystem,
  navigateToDetail,
  navigateBack,
  resourceNameError,
  wireSubmit,
  formFieldHtml,
  formActionsHtml,
  errorBannerHtml,
  loadErrorBanner,
} from './_shared.js';
import { selectableDisks, renderDiskPicker, readSelectedDisks } from './disk-picker.js';

await Promise.all([loadSdk(), loadNlddDesignSystem()]);
await fundament.init;

const content = document.getElementById('content');

let availableDisks = [];
let loadError = null;

try {
  const { items } = await fundament.k8s.list({
    group: 'ceph.fundament.io',
    version: 'v1alpha1',
    resource: 'disks',
  });

  // Only unclaimed, available disks. The DiskInventory reconciler re-runs on
  // DiskPool changes, so a disk another pool just took drops out. null
  // because no pool exists yet at create time.
  availableDisks = selectableDisks(items, null);
} catch (err) {
  loadError = err;
}

if (loadError) {
  content.replaceChildren(loadErrorBanner(`Failed to load disks: ${loadError?.message ?? loadError}`));
} else if (availableDisks.length === 0) {
  content.innerHTML = `
    <nldd-rich-text>
      <p>
        No disks to offer. A disk appears here when it is discovered, unclaimed, and empty
        in the node's last probe. The probe can lag; <code>kubectl get disks</code> shows
        what each disk reports.
      </p>
    </nldd-rich-text>
    <nldd-spacer size="16"></nldd-spacer>
    <nldd-button id="back-btn" type="button" variant="secondary" text="Back to Disk Pools"></nldd-button>
  `;
  document.getElementById('back-btn').addEventListener('click', () => navigateBack());
} else {
  const diskPicker = renderDiskPicker(availableDisks);

  content.innerHTML = `
    <nldd-rich-text>
      <p>
        <strong>Recommendation:</strong> create a single DiskPool per cluster. All pools feed
        one shared Ceph cluster and data is placed across every disk in it, so a second pool
        only contributes more disks.
      </p>
    </nldd-rich-text>
    <nldd-spacer size="12"></nldd-spacer>

    <nldd-form>
      <form id="create-form" novalidate>
        ${errorBannerHtml('error-banner')}

        ${formFieldHtml(
          'Name',
          `<nldd-text-field id="pool-name" name="name" placeholder="default"
                            required maxlength="63" no-spellcheck></nldd-text-field>`,
          { errorId: 'pool-name-error', hint: 'Lowercase letters, digits and dashes.' },
        )}

        <nldd-form-section text="Disks">
          ${diskPicker}
          <nldd-text size="sm" color="secondary">
            Each selected disk joins the shared Ceph cluster, which runs one storage daemon
            (OSD) per disk. Disks spread over two or more nodes let volumes survive a node
            failure. Fundament confirms only that a disk is unclaimed. A disk marked as
            carrying a filesystem holds data, and one marked with nothing may still hold data
            the last probe missed.
          </nldd-text>
        </nldd-form-section>

        ${formActionsHtml({ submitId: 'submit-btn', submitText: 'Create DiskPool', cancelId: 'cancel-btn' })}
      </form>
    </nldd-form>
  `;

  const form = document.getElementById('create-form');
  const nameInput = form.querySelector('[name="name"]');

  document.getElementById('cancel-btn').addEventListener('click', () => navigateBack());

  wireSubmit(form, {
    button: document.getElementById('submit-btn'),
    errorBanner: document.getElementById('error-banner'),
    failPrefix: 'Failed to create DiskPool',
    checks: [
      [nameInput, resourceNameError],
      [
        document.getElementById('disk-picker'),
        () => (readSelectedDisks(form).length === 0 ? 'Please select at least one disk.' : null),
      ],
    ],
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
      navigateToDetail(name);
    },
  });
}
