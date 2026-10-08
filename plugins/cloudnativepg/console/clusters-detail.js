import {
  loadSdk,
  loadNlddDesignSystem,
  escapeHtml,
  navigateBack,
  renderKeyValueList,
  wireSubmit,
} from './_shared.js';
import { CLUSTER_RESOURCE } from './clusters-body.js';

await Promise.all([loadSdk(), loadNlddDesignSystem()]);
const ctx = await fundament.init;

const content = document.getElementById('content');
const heading = document.getElementById('heading');

document.getElementById('back-btn').addEventListener('click', () => navigateBack());

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

// One titled box, the shape of a section on the Console's generated detail page.
function sectionHtml(title, body) {
  return `
    <nldd-box>
      <nldd-container padding="16" md-padding="24">
        <nldd-title size="5"><h2>${escapeHtml(title)}</h2></nldd-title>
        <nldd-spacer size="16"></nldd-spacer>
        ${body}
      </nldd-container>
    </nldd-box>`;
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
    ${sectionHtml('Status', renderKeyValueList(statusPairs, 'Status'))}
    <nldd-spacer size="24"></nldd-spacer>
    ${sectionHtml(
      'Connection',
      `${renderKeyValueList(connectionPairs, 'Connection')}
      <nldd-spacer size="16"></nldd-spacer>
      <nldd-rich-text>
        <p>
          Reachable from pods in the cluster. The Secret also holds a ready-made
          <code>uri</code> for applications.
        </p>
      </nldd-rich-text>
      <nldd-spacer size="16"></nldd-spacer>
      <nldd-form-field label="Read the password">
        <nldd-text-field id="password-cmd" readonly no-spellcheck
                         value="${escapeHtml(passwordCommand(item))}"></nldd-text-field>
      </nldd-form-field>`,
    )}
    <nldd-spacer size="16"></nldd-spacer>
    <nldd-text size="sm" color="secondary">
      This database has one instance and no backups. It cannot be changed from the console.
    </nldd-text>
    <nldd-spacer size="24"></nldd-spacer>
    ${deleteSectionHtml(clusterName)}
  `;
}

// Last on the page, like the Console's own Delete box. Typing the name confirms,
// in place of the Console's modal: a modal centres in the iframe, which is as
// tall as the page, so after scrolling down to here it can open out of view.
function deleteSectionHtml(clusterName) {
  return `
    <nldd-box background="critical">
      <nldd-container padding="16">
        <nldd-title size="5"><h2>Delete ${escapeHtml(clusterName)}</h2></nldd-title>
        <nldd-spacer size="8"></nldd-spacer>
        <nldd-rich-text spacing="flat">
          <p>
            Deleting this database also deletes its storage. It has no backups, so the data
            cannot be recovered.
          </p>
        </nldd-rich-text>
        <nldd-spacer size="16"></nldd-spacer>
        <nldd-form>
          <form id="delete-form" novalidate>
            <nldd-banner id="delete-error" variant="critical" hidden></nldd-banner>
            <nldd-form-field label="Type ${escapeHtml(clusterName)} to confirm">
              <nldd-text-field id="delete-confirm" name="confirm" autocomplete="off"
                               no-spellcheck></nldd-text-field>
              <nldd-validation-list>
                <nldd-validation-item id="delete-confirm-error"></nldd-validation-item>
              </nldd-validation-list>
            </nldd-form-field>
            <nldd-form-actions>
              <nldd-button id="delete-btn" type="submit" variant="destructive" start-icon="delete"
                           text="Delete database"></nldd-button>
            </nldd-form-actions>
          </form>
        </nldd-form>
      </nldd-container>
    </nldd-box>`;
}

function wireDelete(item) {
  const clusterName = item.metadata?.name ?? '';
  wireSubmit(document.getElementById('delete-form'), {
    button: document.getElementById('delete-btn'),
    errorBanner: document.getElementById('delete-error'),
    failPrefix: 'Failed to delete database',
    checks: [
      [
        document.getElementById('delete-confirm'),
        (value) => (value === clusterName ? null : `Type ${clusterName} to confirm.`),
      ],
    ],
    action: async () => {
      await fundament.k8s.delete({
        ...CLUSTER_RESOURCE,
        namespace: item.metadata?.namespace,
        name: clusterName,
      });
      navigateBack();
    },
  });
}

function showError(message) {
  const banner = document.createElement('nldd-banner');
  banner.setAttribute('variant', 'critical');
  banner.setAttribute('text', message);
  content.replaceChildren(banner);
}

if (!name) {
  showError('No database selected.');
} else {
  try {
    const item = await fundament.k8s.get({ ...CLUSTER_RESOURCE, namespace, name });
    heading.textContent = item.metadata?.name ?? name;
    content.innerHTML = render(item);
    wireDelete(item);
    // Select the whole command on focus, so one copy takes all of it. The native
    // input sits in the text field's shadow root.
    const cmd = document.getElementById('password-cmd');
    cmd.addEventListener('focusin', () => cmd.shadowRoot?.querySelector('input')?.select());
  } catch (err) {
    showError(`Failed to load: ${err?.message ?? err}`);
  }
}
