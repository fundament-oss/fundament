import {
  loadSdk,
  navigateToDetail,
  navigateBack,
  replicationFieldHtml,
  resourceNameError,
  wireSubmit,
} from './_shared.js';

await loadSdk();
await fundament.init;

const content = document.getElementById('content');

content.innerHTML = `
  <p class="plugin-text">
    Block storage provides ReadWriteOnce volumes over the shared Ceph cluster's disks.
    It needs at least one StoragePool contributing disks; without one it stays Degraded.
  </p>

  <form id="create-form" class="plugin-form" novalidate>
    <div class="plugin-error" id="error-box" hidden></div>

    <div class="plugin-field">
      <label class="plugin-label" for="bs-name">Name</label>
      <input id="bs-name" name="name" type="text" class="plugin-input"
             placeholder="default" required
             pattern="[a-z0-9]([a-z0-9\\-]*[a-z0-9])?" maxlength="63" />
      <span class="plugin-hint">Lowercase letters, digits and dashes. Names the resulting StorageClass (prefixed ceph-).</span>
    </div>

    ${replicationFieldHtml()}

    <div class="plugin-actions">
      <button id="submit-btn" type="submit" class="plugin-button">Create BlockStorage</button>
      <button id="cancel-btn" type="button" class="plugin-button-secondary">Cancel</button>
    </div>
  </form>
`;

const form = document.getElementById('create-form');
const nameInput = form.querySelector('[name="name"]');

document.getElementById('cancel-btn').addEventListener('click', () => navigateBack());

wireSubmit(form, {
  button: document.getElementById('submit-btn'),
  errorBox: document.getElementById('error-box'),
  busyLabel: 'Creating…',
  failPrefix: 'Failed to create BlockStorage',
  validate: () => {
    const invalid = resourceNameError(nameInput.value.trim());
    if (invalid) nameInput.focus();
    return invalid;
  },
  action: async () => {
    const name = nameInput.value.trim();
    await fundament.k8s.create(
      { group: 'storage.fundament.io', version: 'v1alpha1', resource: 'blockstorages' },
      {
        apiVersion: 'storage.fundament.io/v1alpha1',
        kind: 'BlockStorage',
        metadata: { name },
        spec: { replication: form.querySelector('[name="replication"]').value },
      },
    );
    navigateToDetail(name);
  },
});
