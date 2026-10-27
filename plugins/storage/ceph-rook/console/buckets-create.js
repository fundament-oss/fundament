import { loadSdk, ensureNldd, errorBox, navigateBack, navigateToDetail } from './_shared.js';
import {
  bucketFormHtml,
  hasNoObjectStorage,
  namespaceControlHtml,
  objectStorageClassError,
  objectStorageClasses,
  storeFieldHtml,
  wireBucketForm,
} from './bucket-pages.js';

await loadSdk();
const ctx = await fundament.init;
const content = document.getElementById('content');

// Project members may list StorageClasses. Should the list still be
// forbidden, null means unknown and the form takes the class by name.
async function loadObjectStorageClasses() {
  try {
    const { items } = await fundament.k8s.list({ group: 'storage.k8s.io', version: 'v1', resource: 'storageclasses' });
    return objectStorageClasses(items);
  } catch (err) {
    if (err?.code === 'forbidden') return null;
    throw err;
  }
}

function showForm(classes) {
  content.innerHTML = `
    <nldd-rich-text>
      <p>
        The bucket is provisioned on the object storage you pick. The endpoint
        lands in a ConfigMap and the credentials in a Secret, both named after
        the claim, in the claim's namespace.
      </p>
    </nldd-rich-text>
    <nldd-spacer size="12"></nldd-spacer>
    ${bucketFormHtml({
      namespaceControl: namespaceControlHtml(ctx.namespaces ?? [], ctx.namespaceDisplayNames ?? {}),
      storeField: storeFieldHtml(classes),
    })}
  `;
  const storeInput = content.querySelector('[name="storageClass"]');
  const storeCheck = classes === null
    ? objectStorageClassError
    : (value) => (value ? null : 'Please choose an object storage.');
  wireBucketForm(content, {
    storageClassName: () => storeInput.value.trim(),
    storeChecks: [[storeInput, storeCheck]],
    onCancel: navigateBack,
    onCreated: (_created, name, namespace) => navigateToDetail(name, namespace),
  });
}

try {
  const [classes] = await Promise.all([loadObjectStorageClasses(), ensureNldd()]);
  if (hasNoObjectStorage(classes)) {
    content.innerHTML = '<p class="plugin-text">No object storage is available on this cluster; an organization admin creates it on the cluster page.</p>';
  } else {
    showForm(classes);
  }
} catch (err) {
  content.innerHTML = errorBox(err);
}
