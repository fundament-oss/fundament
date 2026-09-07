import { loadSdk, escapeHtml, navigateToDetail, navigateBack } from './_shared.js';

await loadSdk();
await fundament.init;

const content = document.getElementById('content');

content.innerHTML = `
  <p class="plugin-text">
    File storage provides shared ReadWriteMany volumes over the shared Ceph cluster's disks —
    many pods on many nodes can mount the same volume. It needs at least one StoragePool
    contributing disks; without one it stays Degraded.
  </p>

  <form id="create-form" class="plugin-form" novalidate>
    <div class="plugin-error" id="error-box" hidden></div>

    <div class="plugin-field">
      <label class="plugin-label" for="fs-name">Name</label>
      <input id="fs-name" name="name" type="text" class="plugin-input"
             placeholder="default" required
             pattern="[a-z0-9]([a-z0-9\\-]*[a-z0-9])?" maxlength="63" />
      <span class="plugin-hint">Lowercase letters, digits and dashes. Names the resulting StorageClass (prefixed cephfs-).</span>
    </div>

    <div class="plugin-field">
      <label class="plugin-label" for="replication">Replication</label>
      <select id="replication" name="replication" class="plugin-select">
        <option value="auto" selected>auto (recommended)</option>
        <option value="1">1 — no replication</option>
        <option value="2">2 — two replicas</option>
        <option value="3">3 — three replicas</option>
      </select>
      <span class="plugin-hint">auto derives the replica count from the number of nodes contributing disks to the cluster.</span>
    </div>

    <div class="plugin-field">
      <label class="plugin-label" for="mds-count">Metadata servers</label>
      <input id="mds-count" name="metadataServers" type="number" class="plugin-input"
             min="1" max="5" value="1" />
      <span class="plugin-hint">Active MDS daemons; each gets a standby. 1 is right unless metadata throughput at scale demands more.</span>
    </div>

    <div class="plugin-actions">
      <button id="submit-btn" type="submit" class="plugin-button">Create FileStorage</button>
      <button id="cancel-btn" type="button" class="plugin-button-secondary">Cancel</button>
    </div>
  </form>
`;

const form = document.getElementById('create-form');
const errorBox = document.getElementById('error-box');
const submitBtn = document.getElementById('submit-btn');

function showError(message) {
  errorBox.textContent = message;
  errorBox.hidden = false;
}

document.getElementById('cancel-btn').addEventListener('click', () => navigateBack());

form.addEventListener('submit', async (e) => {
  e.preventDefault();
  errorBox.hidden = true;

  const nameInput = form.querySelector('[name="name"]');
  const name = nameInput.value.trim();
  if (!name) {
    showError('Please enter a name.');
    nameInput.focus();
    return;
  }
  if (!/^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/.test(name)) {
    showError('Use lowercase letters, digits and dashes only.');
    nameInput.focus();
    return;
  }

  const replication = form.querySelector('[name="replication"]').value;

  const metadataServers = Number(form.querySelector('[name="metadataServers"]').value);
  if (!Number.isInteger(metadataServers) || metadataServers < 1 || metadataServers > 5) {
    showError('Metadata servers must be a whole number from 1 to 5.');
    return;
  }

  submitBtn.disabled = true;
  submitBtn.textContent = 'Creating…';

  try {
    await fundament.k8s.create(
      { group: 'storage.fundament.io', version: 'v1alpha1', resource: 'filestorages' },
      {
        apiVersion: 'storage.fundament.io/v1alpha1',
        kind: 'FileStorage',
        metadata: { name },
        spec: { replication, metadataServers },
      },
    );
    navigateToDetail(name);
  } catch (err) {
    showError(`Failed to create FileStorage: ${err?.message ?? err}`);
    submitBtn.disabled = false;
    submitBtn.textContent = 'Create FileStorage';
  }
});
