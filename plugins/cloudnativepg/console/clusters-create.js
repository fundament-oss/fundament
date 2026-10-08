import { loadSdk, loadNlddDesignSystem, escapeHtml, navigateToDetail, navigateBack, wireSubmit } from './_shared.js';
import {
  CLUSTER_RESOURCE,
  POSTGRES_IMAGES,
  DEFAULT_STORAGE_SIZE,
  buildCluster,
  clusterNameError,
  namespaceError,
  storageClassError,
  storageSizeError,
} from './clusters-body.js';

await Promise.all([loadSdk(), loadNlddDesignSystem()]);
const ctx = await fundament.init;

const content = document.getElementById('content');

function isDefaultClass(sc) {
  const annotations = sc.metadata?.annotations ?? {};
  return (
    annotations['storageclass.kubernetes.io/is-default-class'] === 'true' ||
    annotations['storageclass.beta.kubernetes.io/is-default-class'] === 'true'
  );
}

// StorageClasses are cluster-scoped, and only organization admins may list
// them; for a project admin the call is forbidden. null means "unknown", and
// the form then offers only the cluster default. Any other failure throws:
// treating a timeout as "forbidden" would let an organization admin skip the
// choice on a cluster without a default class.
async function loadStorageClasses() {
  try {
    const { items } = await fundament.k8s.list({
      group: 'storage.k8s.io',
      version: 'v1',
      resource: 'storageclasses',
    });
    const classes = items.filter((sc) => sc.metadata?.name);
    // With more than one marked default, Kubernetes gives a PVC without a class
    // the most recently created one, so only that one counts as the default.
    const effectiveDefault = classes
      .filter(isDefaultClass)
      .reduce(
        (newest, sc) =>
          !newest || (sc.metadata.creationTimestamp ?? '') > (newest.metadata.creationTimestamp ?? '') ? sc : newest,
        null,
      );
    return classes
      .map((sc) => ({ name: sc.metadata.name, isDefault: sc === effectiveDefault }))
      .sort((a, b) => a.name.localeCompare(b.name));
  } catch (err) {
    if (err?.code === 'forbidden') return null;
    throw err;
  }
}

// One nldd-form-field. errorId names the empty validation item wireSubmit
// fills when the field's check fails; a field without a check passes none.
function formFieldHtml(label, control, { errorId, hint } = {}) {
  const errors = errorId
    ? `<nldd-validation-list><nldd-validation-item id="${errorId}"></nldd-validation-item></nldd-validation-list>`
    : '';
  const help = hint ? `<nldd-form-field-help-text>${escapeHtml(hint)}</nldd-form-field-help-text>` : '';
  return `
      <nldd-form-field label="${escapeHtml(label)}">
        ${control}
        ${errors}
        ${help}
      </nldd-form-field>`;
}

function optionHtml(value, label, { selected = false, disabled = false } = {}) {
  return `<option value="${escapeHtml(value)}"${selected ? ' selected' : ''}${disabled ? ' disabled' : ''}>${escapeHtml(label)}</option>`;
}

// The console passes the project's namespaces on a create view, by their name
// on the cluster, with the shorter name it shows for each as the label. Without
// them (an organization-level route) the user types one.
function namespaceFieldHtml(namespaces, displayNames = {}) {
  if (Array.isArray(namespaces) && namespaces.length > 0) {
    const options = namespaces.map((n) => optionHtml(n, displayNames[n] ?? n)).join('');
    return formFieldHtml(
      'Namespace',
      `<nldd-dropdown><select id="db-namespace" name="namespace">${options}</select></nldd-dropdown>`,
      { errorId: 'db-namespace-error' },
    );
  }
  return formFieldHtml(
    'Namespace',
    `<nldd-text-field id="db-namespace" name="namespace" placeholder="my-namespace"
                      required maxlength="63" no-spellcheck></nldd-text-field>`,
    { errorId: 'db-namespace-error' },
  );
}

function versionFieldHtml() {
  const options = POSTGRES_IMAGES.map(({ image, label }, i) =>
    optionHtml(image, label, { selected: i === 0 }),
  ).join('');
  return formFieldHtml(
    'PostgreSQL version',
    `<nldd-dropdown><select id="db-version" name="imageName">${options}</select></nldd-dropdown>`,
    { hint: 'The version cannot be changed after creation.' },
  );
}

// Three cases:
// - classes unknown (a project admin): a text field, empty for the default.
// - no default class: a required choice, since "Cluster default" would leave
//   the PVC Pending.
// - a default class: a dropdown that starts on it.
function storageClassFieldHtml(classes) {
  const field = (control, hint) =>
    formFieldHtml('StorageClass', control, { errorId: 'db-storage-class-error', hint });

  if (classes === null) {
    return field(
      `<nldd-text-field id="db-storage-class" name="storageClass" placeholder="Cluster default"
                        maxlength="253" no-spellcheck></nldd-text-field>`,
      'Leave empty for the cluster default. Only an organization admin can list the StorageClasses; ask one for a name if the cluster has no default.',
    );
  }

  const classOptions = classes
    .map((sc) => optionHtml(sc.name, `${sc.name}${sc.isDefault ? ' (default)' : ''}`))
    .join('');
  const defaultName = classes.find((sc) => sc.isDefault)?.name;

  if (!defaultName) {
    const hint =
      classes.length === 0
        ? 'This cluster has no StorageClasses. Ask an organization admin to add one.'
        : 'This cluster has no default StorageClass, so choose one.';
    const placeholder = optionHtml('', 'Choose a StorageClass', { selected: true, disabled: true });
    return field(
      `<nldd-dropdown required><select id="db-storage-class" name="storageClass">${placeholder}${classOptions}</select></nldd-dropdown>`,
      hint,
    );
  }

  const clusterDefault = optionHtml('', `Cluster default (${defaultName})`, { selected: true });
  return field(
    `<nldd-dropdown><select id="db-storage-class" name="storageClass">${clusterDefault}${classOptions}</select></nldd-dropdown>`,
    'Where the data is stored. The cluster default suits most databases.',
  );
}

function showForm(storageClasses) {
  const storageClassRequired = Array.isArray(storageClasses) && !storageClasses.some((sc) => sc.isDefault);

  content.innerHTML = `
    <nldd-rich-text>
      <p>
        Creates a single-instance PostgreSQL database with database <code>app</code>, owned by
        user <code>app</code>. It has no backups. After creation it cannot be changed from the
        console.
      </p>
    </nldd-rich-text>
    <nldd-spacer size="12"></nldd-spacer>

    <nldd-form>
      <form id="create-form" novalidate>
        <nldd-banner id="error-banner" variant="critical" hidden></nldd-banner>

        ${formFieldHtml(
          'Name',
          `<nldd-text-field id="db-name" name="name" placeholder="orders-db"
                            required maxlength="50" no-spellcheck></nldd-text-field>`,
          { errorId: 'db-name-error', hint: 'Starts with a letter; lowercase letters, digits and dashes.' },
        )}

        ${namespaceFieldHtml(ctx.namespaces, ctx.namespaceDisplayNames)}

        ${formFieldHtml(
          'Storage size',
          `<nldd-text-field id="db-size" name="size" value="${escapeHtml(DEFAULT_STORAGE_SIZE)}"
                            required no-spellcheck></nldd-text-field>`,
          {
            errorId: 'db-size-error',
            hint: 'At least 1Gi, for example 1Gi or 20Gi. The size cannot be changed after creation.',
          },
        )}

        ${versionFieldHtml()}

        ${storageClassFieldHtml(storageClasses)}

        <nldd-form-actions>
          <nldd-button-group orientation="horizontal">
            <nldd-button id="submit-btn" type="submit" variant="primary" text="Create database"></nldd-button>
            <nldd-button id="cancel-btn" type="button" variant="secondary" text="Cancel"></nldd-button>
          </nldd-button-group>
        </nldd-form-actions>
      </form>
    </nldd-form>
  `;

  const form = document.getElementById('create-form');
  const field = (name) => form.querySelector(`[name="${name}"]`);

  document.getElementById('cancel-btn').addEventListener('click', () => navigateBack());

  wireSubmit(form, {
    button: document.getElementById('submit-btn'),
    errorBanner: document.getElementById('error-banner'),
    failPrefix: 'Failed to create database',
    checks: [
      [field('name'), clusterNameError],
      [field('namespace'), namespaceError],
      [field('size'), storageSizeError],
      [field('storageClass'), (value) => storageClassError(value, { required: storageClassRequired })],
    ],
    action: async () => {
      const name = field('name').value.trim();
      const namespace = field('namespace').value.trim();
      await fundament.k8s.create(
        { ...CLUSTER_RESOURCE, namespace },
        buildCluster({
          name,
          namespace,
          imageName: field('imageName').value,
          size: field('size').value.trim(),
          storageClass: field('storageClass').value.trim(),
        }),
      );
      navigateToDetail(name, namespace);
    },
  });
}

// undefined: the load failed and the page shows why instead of the form.
let storageClasses;
try {
  storageClasses = await loadStorageClasses();
} catch (err) {
  const banner = document.createElement('nldd-banner');
  banner.setAttribute('variant', 'critical');
  banner.setAttribute('text', `Failed to load StorageClasses: ${err?.message ?? err}. Reload the page to try again.`);
  content.replaceChildren(banner);
}
if (storageClasses !== undefined) showForm(storageClasses);
