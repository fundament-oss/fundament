// Page factory for the consumer kinds (BlockStorage, FileStorage,
// ObjectStorage): one implementation of the list, detail, create and edit
// pages, parameterized per kind. Create and edit are pages of their own that
// the list and detail pages open in a console sheet (fundament.openSheet). The
// kinds differ only in labels/texts and a few kind-specific spec fields, so
// each page file reduces to a factory call with its kind's config.

import {
  bootPage,
  emptyStateHtml,
  errorBox,
  fetchCephCapacity,
  freeSpace,
  addInfoButton,
  REPLICAS_INFO,
  latestRead,
  loadingHtml,
  navigateToDetail,
  phaseBadgeHtml,
  renderKeyValueList,
  resourceListHtml,
  sectionHtml,
  wireListRows,
  replicasFieldHtml,
  replicasValue,
  defaultFieldHtml,
  integerFieldHtml,
  integerFieldError,
  resourceNameError,
  wireSubmit,
  selectedDiskCount,
  formFieldHtml,
  formActionsHtml,
  errorBannerHtml,
} from './_shared.js';
import { mountBucketsSection, OBJECT_STORAGE_CLASS_PREFIX, OBJECT_STORAGE_NAME_MAX } from './bucket-pages.js';

const GROUP_VERSION = { group: 'ceph.fundament.io', version: 'v1alpha1' };

// Descriptors for the kind-specific integer spec fields. Everywhere a field
// surfaces — forms, validation, spec building, list cells, detail pairs —
// iterates cfg.intFields, so a field cannot be rendered in one form and
// forgotten in another.
const METADATA_SERVERS = {
  name: 'metadataServers',
  id: 'mds-count',
  label: 'Metadata servers',
  hint: 'Each gets a standby that takes over if it fails.',
  min: 1,
  max: 5,
  default: 1,
};

const GATEWAY_INSTANCES = {
  name: 'gatewayInstances',
  id: 'rgw-count',
  label: 'Gateway instances',
  hint: 'Pods that serve the S3 API. With 1, the S3 API is down while that pod restarts.',
  min: 1,
  max: 5,
  default: 1,
};

export const BLOCKSTORAGE = {
  resource: 'blockstorages',
  kind: 'BlockStorage',
  label: 'Block Storage',
  labelMidSentence: 'block storage',
  emptyMessage: 'No block storage yet',
  noneSelected: 'No block storage selected.',
  storageClassPrefix: 'ceph-',
  // The info box on the list and detail pages.
  about: [
    'Block storage gives a pod its own disk-like volume (ReadWriteOnce), for example for a database.',
    'It needs a disk pool with disks; without one it stays Degraded.',
    REPLICAS_INFO,
  ],
  intFields: [],
  defaultToggle: true,
  nameMaxLength: 63,
};

export const FILESTORAGE = {
  resource: 'filestorages',
  kind: 'FileStorage',
  label: 'File Storage',
  labelMidSentence: 'file storage',
  emptyMessage: 'No file storage yet',
  noneSelected: 'No file storage selected.',
  storageClassPrefix: 'cephfs-',
  about: [
    'File storage gives volumes that many pods on different nodes can use at the same time (ReadWriteMany), for example for shared files.',
    'It needs a disk pool with disks; without one it stays Degraded.',
    'Metadata servers keep track of files and folders. Each one gets a standby that takes over when it fails.',
    REPLICAS_INFO,
  ],
  intFields: [METADATA_SERVERS],
  defaultToggle: false,
  // The CRD caps FileStorage names at 56: Rook derives a cephfs-<name> label
  // capped at 63. Enforced here too so the form rejects it before the server.
  nameMaxLength: 56,
};

export const OBJECTSTORAGE = {
  resource: 'objectstorages',
  kind: 'ObjectStorage',
  label: 'Object Storage',
  labelMidSentence: 'object storage',
  emptyMessage: 'No object storage yet',
  noneSelected: 'No object storage selected.',
  storageClassPrefix: OBJECT_STORAGE_CLASS_PREFIX,
  about: [
    'Object storage gives S3-compatible buckets that apps reach over HTTP. Create buckets on this storage\'s page or on a project\'s Buckets page.',
    'It needs a disk pool with disks; without one it stays Degraded.',
    REPLICAS_INFO,
  ],
  intFields: [GATEWAY_INSTANCES],
  defaultToggle: false,
  // The CRD caps ObjectStorage names at 30: Rook rejects CephObjectStore
  // names over 38 and the derived name is cephobj-<name>. Enforced here too
  // so the form rejects it first.
  nameMaxLength: OBJECT_STORAGE_NAME_MAX,
  // The detail page embeds the Buckets section: the ObjectBucketClaims
  // provisioned against this store's StorageClass.
  detailSection: mountBucketsSection,
};

function intFieldValue(form, field) {
  return Number(form.querySelector(`[name="${field.name}"]`).value);
}

// intFieldChecks validates the kind's integer fields, each under its own field.
function intFieldChecks(cfg, form) {
  return cfg.intFields.map((field) => [
    form.querySelector(`[name="${field.name}"]`),
    (value) => integerFieldError(field, Number(value)),
  ]);
}

// intFieldsHtml renders the kind's integer inputs for a create or edit form;
// spec is undefined on create, so every field falls back to its default.
function intFieldsHtml(cfg, spec) {
  return cfg.intFields
    .map((field) => integerFieldHtml(field, spec?.[field.name] ?? field.default))
    .join('');
}

// specFrom reads the create/edit form into a spec object.
function specFrom(cfg, form) {
  const spec = { replicas: replicasValue(form) };
  for (const field of cfg.intFields) spec[field.name] = intFieldValue(form, field);
  // Always sent, so unticking the box merge-patches the field back to false.
  if (cfg.defaultToggle) spec.default = form.querySelector('[name="default"]').checked;
  return spec;
}

// The operator sets the failure domain to host only when the copies land on
// two or more nodes (ComputeReplication); osd keeps them on one node's disks.
export function copiesOnSeparateNodes(failureDomain) {
  if (failureDomain === 'host') return 'Yes';
  if (failureDomain === 'osd') return 'No';
  return failureDomain ?? '—';
}

export async function consumerListPage(cfg) {
  const list = document.getElementById('list');
  list.innerHTML = loadingHtml(`Loading ${cfg.labelMidSentence}…`);

  await bootPage();

  document.getElementById('create-btn').addEventListener('click', () => {
    fundament.openSheet({ view: 'create' });
  });
  addInfoButton(document.getElementById('page-actions'), `About ${cfg.labelMidSentence}`, cfg.about);
  const nextRead = latestRead();
  fundament.onResourcesChanged(() => renderList(list, cfg, nextRead()));

  await renderList(list, cfg, nextRead());
}

// "StorageClass ceph-default · 2 replicas · default", and why a resource is
// not Ready when its status says so. The failure domain stays on the detail
// page, as whether the copies are on separate nodes.
function summary(item) {
  const status = item.status ?? {};
  const parts = [`StorageClass ${status.storageClassName ?? '—'}`];
  if (status.replicas) parts.push(`${status.replicas} ${status.replicas === 1 ? 'replica' : 'replicas'}`);
  if (item.spec?.default) parts.push('default');
  if (status.message) parts.push(status.message);
  return parts.join(' · ');
}

// The disk pools, or undefined when they cannot be read: the empty state then
// says nothing about them rather than guessing.
async function listDiskPools() {
  try {
    const { items } = await fundament.k8s.list({ ...GROUP_VERSION, resource: 'diskpools' });
    return items ?? [];
  } catch {
    return undefined;
  }
}

// What an empty list asks for: a disk pool with disks while every pool is
// known to have none, else the resource itself. A pool not counted yet may
// still get disks, so it does not count as one without.
export function emptySupportingText(cfg, pools) {
  const noDisks = pools?.every((pool) => selectedDiskCount(pool.status) === 0);
  return noDisks ? 'Create a disk pool with disks first.' : `Use New ${cfg.labelMidSentence}… to create one.`;
}

// Fills the list; again whenever a sheet over the list saved. Renders only
// while isCurrent says no later read has started (see latestRead).
async function renderList(list, cfg, isCurrent) {
  try {
    const { items } = await fundament.k8s.list({ ...GROUP_VERSION, resource: cfg.resource });
    if (!isCurrent()) return;

    if (!items || items.length === 0) {
      const pools = await listDiskPools();
      if (!isCurrent()) return;
      list.innerHTML = emptyStateHtml({
        icon: 'cylinder-split-slash',
        text: cfg.emptyMessage,
        supportingText: emptySupportingText(cfg, pools),
      });
      return;
    }
    list.innerHTML = resourceListHtml(
      items.map((item) => ({
        name: item.metadata?.name ?? '',
        supportingText: summary(item),
        badge: phaseBadgeHtml(item.status?.phase),
      })),
      cfg.label,
    );
    wireListRows(list, navigateToDetail);
  } catch (err) {
    if (!isCurrent()) return;
    list.innerHTML = errorBox(err);
  }
}

export async function consumerDetailPage(cfg) {
  const content = document.getElementById('content');
  const actions = document.getElementById('actions');
  content.innerHTML = loadingHtml();

  const ctx = await bootPage();

  const resource = { ...GROUP_VERSION, resource: cfg.resource };
  const name = ctx.resource?.name;

  function renderReadOnly(item, cap) {
    const status = item.status ?? {};

    // The phase is the badge beside the Console's title; the message says why.
    const pairs = [
      ['StorageClass', status.storageClassName ?? '—'],
      ['Replicas', String(status.replicas ?? '—')],
    ];
    for (const field of cfg.intFields) {
      pairs.push([field.label, String(item.spec?.[field.name] ?? field.default)]);
    }
    if (cfg.defaultToggle) pairs.push(['Default StorageClass', item.spec?.default ? 'Yes' : 'No']);
    pairs.push(['Copies on separate nodes', copiesOnSeparateNodes(status.failureDomain)]);
    const free = freeSpace(cap, status.replicas);
    if (free) pairs.push(free);
    if (status.message) pairs.push(['Message', status.message]);

    return sectionHtml('Status', renderKeyValueList(pairs, 'Status'));
  }

  const nextRead = latestRead();
  // The kind-specific extra section (e.g. ObjectStorage's Buckets) lives in
  // #extra, outside #content, so showDetail's re-renders never touch it. It is
  // mounted once, by the first read that renders.
  const extra = document.getElementById('extra');
  let extraMounted = false;

  async function showDetail() {
    const isCurrent = nextRead();
    try {
      // Users cannot run ceph df, but the CephCluster status carries the raw
      // numbers. fetchCephCapacity never throws: without them the row is left out.
      const [item, cap] = await Promise.all([
        fundament.k8s.get({ ...resource, name }),
        fetchCephCapacity(),
      ]);
      if (!isCurrent()) return;
      content.innerHTML = renderReadOnly(item, cap);
      if (!extraMounted && extra && cfg.detailSection) {
        extraMounted = true;
        cfg.detailSection(extra, item, ctx, cfg);
      }
      actions.hidden = false;
      // .onclick, not addEventListener: this button lives outside #content and
      // survives every re-render, so listeners would stack. (CSP restricts inline
      // handler *attributes*, not this.)
      document.getElementById('edit-btn').onclick = () => {
        fundament.openSheet({ view: 'edit', resource: { name } });
      };
    } catch (err) {
      if (!isCurrent()) return;
      actions.hidden = true;
      content.innerHTML = errorBox(err);
    }
  }

  if (!name) {
    content.innerHTML = errorBox(cfg.noneSelected, 'Cannot show this page');
    return;
  }
  // In the actions group, which shows once the resource has loaded.
  addInfoButton(document.getElementById('page-actions'), `About ${cfg.labelMidSentence}`, cfg.about);
  // The edit sheet saved: show what it saved.
  fundament.onResourcesChanged(() => showDetail());
  await showDetail();
}

// The edit form, which the detail page opens in a sheet (see sheet.js).
// Saving merge-patches spec only and closes the sheet, and the detail page
// reads the resource again.
export async function consumerEditPage(cfg, ctx) {
  const body = document.getElementById('content');
  const resource = { ...GROUP_VERSION, resource: cfg.resource };
  const name = ctx.resource?.name;
  if (!name) {
    body.innerHTML = errorBox(cfg.noneSelected, 'Cannot edit');
    return;
  }

  let item;
  try {
    item = await fundament.k8s.get({ ...resource, name });
  } catch (err) {
    body.innerHTML = errorBox(err);
    return;
  }

  body.innerHTML = `
    <nldd-form>
      <form novalidate>
        ${errorBannerHtml('edit-error')}

        ${replicasFieldHtml(item.spec?.replicas)}

        ${intFieldsHtml(cfg, item.spec)}

        ${cfg.defaultToggle ? defaultFieldHtml(item.spec?.default === true) : ''}

        ${formActionsHtml({ submitId: 'save-btn', submitText: 'Save', cancelId: 'cancel-btn' })}
      </form>
    </nldd-form>
  `;

  const form = body.querySelector('form');
  body.querySelector('#cancel-btn').addEventListener('click', () => fundament.closeSheet());

  wireSubmit(form, {
    button: body.querySelector('#save-btn'),
    errorBanner: body.querySelector('#edit-error'),
    failPrefix: 'Failed to save',
    checks: intFieldChecks(cfg, form),
    action: async () => {
      // Merge-patch of spec only: status is untouched.
      await fundament.k8s.patch({ ...resource, name }, { spec: specFrom(cfg, form) });
      fundament.closeSheet({ changed: true });
    },
  });
}

// The create form, which the list page opens in a sheet (see sheet.js). Also
// the routed create page: closeSheet goes back to the list there, and
// navigateToDetail shows the new resource either way.
export function consumerCreatePage(cfg) {
  const body = document.getElementById('content');
  body.innerHTML = `
    <nldd-form>
      <form novalidate>
        ${errorBannerHtml('error-banner')}

        ${formFieldHtml(
          'Name',
          `<nldd-text-field id="consumer-name" name="name" placeholder="default"
                            required maxlength="${cfg.nameMaxLength}" no-spellcheck></nldd-text-field>`,
          {
            errorId: 'consumer-name-error',
            hint: `Lowercase letters, digits and dashes. The StorageClass becomes ${cfg.storageClassPrefix}<name>.`,
          },
        )}

        ${replicasFieldHtml()}

        ${intFieldsHtml(cfg)}

        ${cfg.defaultToggle ? defaultFieldHtml() : ''}

        ${formActionsHtml({ submitId: 'submit-btn', submitText: `Create ${cfg.labelMidSentence}`, cancelId: 'cancel-btn' })}
      </form>
    </nldd-form>
  `;

  const form = body.querySelector('form');
  const nameInput = form.querySelector('[name="name"]');

  body.querySelector('#cancel-btn').addEventListener('click', () => fundament.closeSheet());

  wireSubmit(form, {
    button: body.querySelector('#submit-btn'),
    errorBanner: body.querySelector('#error-banner'),
    failPrefix: `Failed to create ${cfg.labelMidSentence}`,
    checks: [
      [nameInput, (value) => resourceNameError(value, cfg.nameMaxLength)],
      ...intFieldChecks(cfg, form),
    ],
    action: async () => {
      const name = nameInput.value.trim();
      await fundament.k8s.create(
        { ...GROUP_VERSION, resource: cfg.resource },
        {
          apiVersion: 'ceph.fundament.io/v1alpha1',
          kind: cfg.kind,
          metadata: { name },
          spec: specFrom(cfg, form),
        },
      );
      // From a sheet the console closes it and shows the new resource.
      navigateToDetail(name);
    },
  });
}
