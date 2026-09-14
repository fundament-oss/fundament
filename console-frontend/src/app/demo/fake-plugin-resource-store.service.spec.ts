import { TestBed } from '@angular/core/testing';
import * as fx from './fixtures';
import FakePluginResourceStoreService from './fake-plugin-resource-store.service';

const certificateCrd = fx.pluginCrds['cert-manager'][0];

// Guards the walkthrough slides that show a plugin's own screens. Routes name a
// plugin by its installation name, so the store is handed "system--cert-manager"
// where the fixtures are keyed "cert-manager" — a mismatch that empties the list
// and the detail page without failing anything.
describe('demo plugin resources', () => {
  let store: FakePluginResourceStoreService;

  beforeEach(() => {
    TestBed.configureTestingModule({});
    store = TestBed.inject(FakePluginResourceStoreService);
  });

  it("lists a plugin's objects by installation name", async () => {
    const resources = await store.loadResources(
      certificateCrd,
      'cl-production',
      'https://kube-api-proxy.example',
      'system--cert-manager',
    );

    expect(resources.map((r) => r.metadata.name)).toEqual([
      'burgerzaken-portaal',
      'burgerzaken-api',
      'burgerzaken-afspraken',
    ]);
  });

  it('gets the certificate the detail slide opens', async () => {
    const resource = await store.loadResource(
      certificateCrd,
      'cl-production',
      'https://kube-api-proxy.example',
      'system--cert-manager',
      'burgerzaken-portaal',
      undefined,
    );

    expect(resource?.spec?.['secretName']).toBe('burgerzaken-portaal-tls');
  });

  it('has nothing for an installation no definition claims', async () => {
    const resources = await store.loadResources(
      certificateCrd,
      'cl-production',
      'https://kube-api-proxy.example',
      // The catalog name, which is what the routes wrongly carried before.
      'cert-manager',
    );

    expect(resources).toEqual([]);
  });
});
