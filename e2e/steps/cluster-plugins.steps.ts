/**
 * Steps for the @cluster-plugins feature: the plugin machinery on a real
 * cluster, and a plugin installed, served and uninstalled there. Runs in the
 * test-env VM (`just test-env test`).
 */

import { Given, When, Then } from '@cucumber/cucumber';
import { expect } from '@playwright/test';
import { ICustomWorld } from '../support/world.ts';
import { fixtureClusterId, fixturePlugin, session } from '../support/fixtures.ts';
import { withClusterAccess } from '../support/kube.ts';
import { getInstallation, installationName, installPlugin, waitForGone, waitForPhase } from '../support/plugins.ts';
import { pollUntil } from '../support/poll.ts';
import { withShootAdmin } from '../support/shoot.ts';

const { organizationName } = fixturePlugin;

Then('the baseline cluster runs the plugin controller', { timeout: 120_000 }, async function (this: ICustomWorld) {
  await withShootAdmin(await fixtureClusterId(this), async (kubectl) => {
    const available = kubectl([
      'get', 'deployment', 'plugin-controller', '-n', 'fundament-system',
      '-o', 'jsonpath={.status.availableReplicas}',
    ]);
    expect(Number(available), 'available plugin-controller replicas').toBeGreaterThan(0);
  });
});

Given(
  'the plugin {string} is not installed on the baseline cluster',
  { timeout: 6 * 60_000 },
  async function (this: ICustomWorld, plugin: string) {
    const name = installationName(organizationName, plugin);
    await withClusterAccess(session(this), await fixtureClusterId(this), async (kubectl) => {
      if (getInstallation(kubectl, name)) {
        kubectl(['delete', 'plugininstallations.plugins.fundament.io', name, '--wait=false']);
      }
      await waitForGone(kubectl, name, 5 * 60_000);
    });
  },
);

When(
  'I install the plugin {string} on the baseline cluster',
  { timeout: 120_000 },
  async function (this: ICustomWorld, plugin: string) {
    await withClusterAccess(session(this), await fixtureClusterId(this), (kubectl) =>
      installPlugin(session(this), kubectl, organizationName, plugin),
    );
  },
);

Then(
  'within {int} minutes the plugin {string} is running',
  { timeout: -1 },
  async function (this: ICustomWorld, minutes: number, plugin: string) {
    const name = installationName(organizationName, plugin);
    const installation = await withClusterAccess(session(this), await fixtureClusterId(this), (kubectl) =>
      waitForPhase(kubectl, name, 'Running', minutes * 60_000),
    );
    this.testData.pluginVersion = installation.spec.definitionRef.pluginVersion;
  },
);

// Any page of the plugin's console bundle; cert-manager's has no index. The
// path names the installation, as the console builds it
// (plugin-console-url.utils.ts).
Then(
  'the console of the plugin {string} is served through the plugin proxy',
  { timeout: 120_000 },
  async function (this: ICustomWorld, plugin: string) {
    const clusterId = await fixtureClusterId(this);
    const name = installationName(organizationName, plugin);
    const url = `${this.pluginProxyUrl}/clusters/${clusterId}/plugins/${name}/${this.testData.pluginVersion}/console/certificates-list.html`;
    // The proxy reaches the new pod through the shoot's API server, which may
    // take a moment after the installation reports Running.
    await pollUntil(60_000, `the console of ${plugin}`, async () => {
      const response = await fetch(url, { headers: { Cookie: `fundament_auth=${this.authToken}` } });
      return response.status === 200 ? response : undefined;
    });
  },
);

When('I uninstall the plugin {string}', { timeout: 120_000 }, async function (this: ICustomWorld, plugin: string) {
  await withClusterAccess(session(this), await fixtureClusterId(this), async (kubectl) => {
    kubectl(['delete', 'plugininstallations.plugins.fundament.io', installationName(organizationName, plugin), '--wait=false']);
  });
});

Then(
  'within {int} minutes the plugin {string} and its namespace are gone',
  { timeout: -1 },
  async function (this: ICustomWorld, minutes: number, plugin: string) {
    await withClusterAccess(session(this), await fixtureClusterId(this), (kubectl) =>
      waitForGone(kubectl, installationName(organizationName, plugin), minutes * 60_000),
    );
  },
);
