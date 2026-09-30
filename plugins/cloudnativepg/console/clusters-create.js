import { loadSdk, escapeHtml, navigateToDetail, navigateBack, wireSubmit } from './_shared.js';
import {
  CLUSTER_RESOURCE,
  POSTGRES_IMAGES,
  DEFAULT_STORAGE_SIZE,
  buildCluster,
  clusterNameError,
  namespaceError,
  storageSizeError,
} from './clusters-body.js';

await loadSdk();
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
// the form then offers only the cluster default.
async function loadStorageClasses() {
  try {
    const { items } = await fundament.k8s.list({
      group: 'storage.k8s.io',
      version: 'v1',
      resource: 'storageclasses',
    });
    return items
      .filter((sc) => sc.metadata?.name)
      .map((sc) => ({ name: sc.metadata.name, isDefault: isDefaultClass(sc) }))
      .sort((a, b) => a.name.localeCompare(b.name));
  } catch {
    return null;
  }
}

// The console passes the project's namespaces on a create view. Without them
// (an organization-level route) the user types one.
function namespaceFieldHtml(namespaces) {
  if (Array.isArray(namespaces) && namespaces.length > 0) {
    const options = namespaces
      .map((n) => `<option value="${escapeHtml(n)}">${escapeHtml(n)}</option>`)
      .join('');
    return `
      <div class="plugin-field">
        <label class="plugin-label" for="db-namespace">Namespace</label>
        <select id="db-namespace" name="namespace" class="plugin-select">${options}</select>
      </div>`;
  }
  return `
    <div class="plugin-field">
      <label class="plugin-label" for="db-namespace">Namespace</label>
      <input id="db-namespace" name="namespace" type="text" class="plugin-input"
             placeholder="my-namespace" required
             pattern="[a-z0-9]([a-z0-9\\-]*[a-z0-9])?" maxlength="63" />
    </div>`;
}

function versionFieldHtml() {
  const options = POSTGRES_IMAGES.map(
    ({ image, label }, i) =>
      `<option value="${escapeHtml(image)}"${i === 0 ? ' selected' : ''}>${escapeHtml(label)}</option>`,
  ).join('');
  return `
    <div class="plugin-field">
      <label class="plugin-label" for="db-version">PostgreSQL version</label>
      <select id="db-version" name="imageName" class="plugin-select">${options}</select>
      <span class="plugin-hint">The version cannot be changed after creation.</span>
    </div>`;
}

function storageClassFieldHtml(classes) {
  const defaultName = classes?.find((sc) => sc.isDefault)?.name;
  const clusterDefault = defaultName ? `Cluster default (${defaultName})` : 'Cluster default';
  const options = [`<option value="" selected>${escapeHtml(clusterDefault)}</option>`]
    .concat(
      (classes ?? []).map(
        (sc) => `<option value="${escapeHtml(sc.name)}">${escapeHtml(sc.name)}${sc.isDefault ? ' (default)' : ''}</option>`,
      ),
    )
    .join('');
  const hint =
    classes === null
      ? 'Only an organization admin can see the other StorageClasses, so this uses the cluster default.'
      : 'Where the data is stored. The cluster default suits most databases.';
  return `
    <div class="plugin-field">
      <label class="plugin-label" for="db-storage-class">StorageClass</label>
      <select id="db-storage-class" name="storageClass" class="plugin-select">${options}</select>
      <span class="plugin-hint">${escapeHtml(hint)}</span>
    </div>`;
}

const storageClasses = await loadStorageClasses();

content.innerHTML = `
  <p class="plugin-text">
    Creates a single-instance PostgreSQL database with database <code>app</code>, owned by
    user <code>app</code>. It has no backups. After creation it cannot be changed or deleted
    from the console.
  </p>

  <form id="create-form" class="plugin-form" novalidate>
    <div class="plugin-error" id="error-box" hidden></div>

    <div class="plugin-field">
      <label class="plugin-label" for="db-name">Name</label>
      <input id="db-name" name="name" type="text" class="plugin-input"
             placeholder="orders-db" required
             pattern="[a-z]([a-z0-9\\-]*[a-z0-9])?" maxlength="50" />
      <span class="plugin-hint">Starts with a letter; lowercase letters, digits and dashes.</span>
    </div>

    ${namespaceFieldHtml(ctx.namespaces)}

    <div class="plugin-field">
      <label class="plugin-label" for="db-size">Storage size</label>
      <input id="db-size" name="size" type="text" class="plugin-input"
             value="${escapeHtml(DEFAULT_STORAGE_SIZE)}" required />
      <span class="plugin-hint">For example 1Gi or 500Mi. The size cannot be changed after creation.</span>
    </div>

    ${versionFieldHtml()}

    ${storageClassFieldHtml(storageClasses)}

    <div class="plugin-actions">
      <button id="submit-btn" type="submit" class="plugin-button">Create database</button>
      <button id="cancel-btn" type="button" class="plugin-button-secondary">Cancel</button>
    </div>
  </form>
`;

const form = document.getElementById('create-form');
const field = (name) => form.querySelector(`[name="${name}"]`);

document.getElementById('cancel-btn').addEventListener('click', () => navigateBack());

wireSubmit(form, {
  button: document.getElementById('submit-btn'),
  errorBox: document.getElementById('error-box'),
  busyLabel: 'Creating…',
  failPrefix: 'Failed to create database',
  validate: () => {
    for (const [name, check] of [
      ['name', clusterNameError],
      ['namespace', namespaceError],
      ['size', storageSizeError],
    ]) {
      const invalid = check(field(name).value.trim());
      if (invalid) {
        field(name).focus();
        return invalid;
      }
    }
    return null;
  },
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
        storageClass: field('storageClass').value,
      }),
    );
    navigateToDetail(name, namespace);
  },
});
