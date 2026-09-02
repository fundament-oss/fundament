import { create } from '@bufbuild/protobuf';
import { createClient } from '@connectrpc/connect';
import {
  CatalogService,
  ListPluginVersionsRequestSchema,
} from '../../generated/catalog/v1/catalog_pb';
import createDemoTransport from './mock-transport';
import * as fx from './fixtures';

// Guards the walkthrough's install slide: the install modal loads a plugin's
// published versions before it can install anything, and an RPC the demo transport
// does not answer leaves the modal stuck on "Couldn't load versions".
describe('demo plugin versions', () => {
  const client = createClient(CatalogService, createDemoTransport());

  it('lists the catalog plugin versions, latest first', async () => {
    const resp = await client.listPluginVersions(
      create(ListPluginVersionsRequestSchema, { pluginId: 'pl-cert-manager' }),
    );

    expect(resp.versions.length).toBeGreaterThan(1);
    expect(resp.versions[0].version).toBe('v1.17.2');
    expect(resp.versions.every((v) => v.definitionHash.startsWith('sha256:'))).toBe(true);
  });

  it('pins every catalog plugin to a version it also publishes', async () => {
    await Promise.all(
      fx.plugins.map(async (plugin) => {
        const resp = await client.listPluginVersions(
          create(ListPluginVersionsRequestSchema, { pluginId: plugin.id }),
        );

        // The card advertises the latest published version, and the modal defaults
        // to it — the two must agree or a fresh install pins a version that is not
        // on offer.
        expect(resp.versions[0]?.version).toBe(plugin.pluginVersion);
        expect(resp.versions[0]?.definitionHash).toBe(plugin.definitionHash);
      }),
    );
  });

  it('answers with nothing published for an unknown plugin', async () => {
    const resp = await client.listPluginVersions(
      create(ListPluginVersionsRequestSchema, { pluginId: 'pl-does-not-exist' }),
    );

    expect(resp.versions).toEqual([]);
  });
});
