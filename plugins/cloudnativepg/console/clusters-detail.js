import { loadSdk, escapeHtml, renderDefList } from './_shared.js';
import { CLUSTER_RESOURCE } from './clusters-body.js';

await loadSdk();
const ctx = await fundament.init;

const content = document.getElementById('content');
const heading = document.getElementById('heading');

const name = ctx.resource?.name;
const namespace = ctx.resource?.namespace;

function readyText(item) {
  const ready = item.status?.readyInstances ?? 0;
  const total = item.spec?.instances ?? item.status?.instances ?? 1;
  return `${ready} of ${total}`;
}

// The page never reads the Secret, so no password reaches the UI; it shows the
// command that does. Plain text in a read-only field: the iframe has no
// clipboard-write permission, so a Copy button could not work.
function passwordCommand(item) {
  const ns = item.metadata?.namespace ?? '';
  const secret = `${item.metadata?.name ?? ''}-app`;
  return `kubectl -n ${ns} get secret ${secret} -o jsonpath='{.data.password}' | base64 -d`;
}

function render(item) {
  const ns = item.metadata?.namespace ?? '';
  const clusterName = item.metadata?.name ?? '';
  const status = item.status ?? {};
  const spec = item.spec ?? {};

  const statusPairs = [
    ['Phase', status.phase ?? 'Unknown'],
    ['Ready instances', readyText(item)],
    ['Image', spec.imageName ?? status.image ?? 'Unknown'],
    ['Storage', spec.storage?.size ?? 'Unknown'],
    ['StorageClass', spec.storage?.storageClass ?? 'Cluster default'],
  ];

  const connectionPairs = [
    ['Host', `${clusterName}-rw.${ns}.svc`],
    ['Port', '5432'],
    ['Database', 'app'],
    ['User', 'app'],
    ['Credentials Secret', `${clusterName}-app`],
  ];

  return `
    <h2 class="plugin-heading">Status</h2>
    ${renderDefList(statusPairs)}

    <h2 class="plugin-heading">Connection</h2>
    ${renderDefList(connectionPairs)}
    <p class="plugin-hint">
      Reachable from pods in the cluster. The Secret also holds a ready-made
      <code>uri</code> for applications.
    </p>

    <div class="plugin-field">
      <label class="plugin-label" for="password-cmd">Read the password</label>
      <input id="password-cmd" type="text" class="plugin-input" readonly
             value="${escapeHtml(passwordCommand(item))}" />
    </div>

    <p class="plugin-hint">
      This database has one instance and no backups. It cannot be changed or deleted from
      the console.
    </p>
  `;
}

if (!name) {
  content.textContent = 'No database selected.';
} else {
  try {
    const item = await fundament.k8s.get({ ...CLUSTER_RESOURCE, namespace, name });
    heading.textContent = `Database · ${item.metadata?.name ?? name}`;
    content.innerHTML = render(item);
    const cmd = document.getElementById('password-cmd');
    cmd.addEventListener('focus', () => cmd.select());
  } catch (err) {
    content.innerHTML = `<div class="plugin-error">${escapeHtml(
      `Failed to load: ${err?.message ?? err}`,
    )}</div>`;
  }
}
