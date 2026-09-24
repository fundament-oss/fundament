import { TestBed } from '@angular/core/testing';
import { vi } from 'vitest';
import { from, of } from 'rxjs';
import { create } from '@bufbuild/protobuf';
import InstallPluginModalComponent, { InstallSelection } from './install-plugin-modal';
import { CATALOG } from '../../connect/tokens';
import type { ObservableClient } from '../../connect/observable-client';
import {
  CatalogService,
  ConfigSchemaEntrySchema,
  ConfigType,
  type GetPluginDefinitionResponse,
  GetPluginDefinitionResponseSchema,
} from '../../generated/catalog/v1/catalog_pb';

function build(getPluginDefinition: ReturnType<typeof vi.fn>) {
  TestBed.configureTestingModule({
    providers: [
      {
        provide: CATALOG,
        useValue: {
          getPluginDefinition,
        } as unknown as ObservableClient<typeof CatalogService>,
      },
    ],
  });
  const fixture = TestBed.createComponent(InstallPluginModalComponent);
  fixture.componentRef.setInput('organizationName', 'acme');
  fixture.componentRef.setInput('pluginName', 'ceph-rook');
  fixture.detectChanges();
  return fixture.componentInstance;
}

describe('InstallPluginModalComponent onInstallOne', () => {
  it('empty schema installs immediately without showing the form', async () => {
    const getPluginDefinition = vi.fn().mockReturnValue(
      of(create(GetPluginDefinitionResponseSchema, { configSchema: [] })),
    );
    const component = build(getPluginDefinition);

    let emitted: InstallSelection | undefined;
    component.install.subscribe((value) => {
      emitted = value;
    });

    await component.onInstallOne('c1', { version: 'v1.0.0', hash: 'sha256:abc' });

    expect(emitted).toEqual({
      clusterIds: ['c1'],
      version: 'v1.0.0',
      hash: 'sha256:abc',
      config: {},
    });
    expect(component.pendingInstall()).toBeNull();
  });

  it('non-empty schema shows the form instead of installing', async () => {
    const getPluginDefinition = vi.fn().mockReturnValue(
      of(
        create(GetPluginDefinitionResponseSchema, {
          configSchema: [
            create(ConfigSchemaEntrySchema, {
              name: 'MON_COUNT',
              type: ConfigType.INT,
              defaultValue: '3',
            }),
          ],
        }),
      ),
    );
    const component = build(getPluginDefinition);

    const install = vi.fn();
    component.install.subscribe(install);

    await component.onInstallOne('c1', { version: 'v1.0.0', hash: 'sha256:abc' });

    expect(install).not.toHaveBeenCalled();
    expect(component.pendingInstall()).toEqual({
      clusterId: 'c1',
      option: { version: 'v1.0.0', hash: 'sha256:abc' },
      schema: [
        expect.objectContaining({ name: 'MON_COUNT', type: ConfigType.INT, defaultValue: '3' }),
      ],
    });
  });

  // The component is never destroyed (both parents render it unconditionally),
  // so a fetch outliving the sheet it was started from is a real race, not a
  // hypothetical one.
  it('drops a schema fetch that resolves after the sheet was closed', async () => {
    let resolveFetch: (value: GetPluginDefinitionResponse) => void = () => {};
    const deferred = new Promise<GetPluginDefinitionResponse>((resolve) => {
      resolveFetch = resolve;
    });
    const getPluginDefinition = vi.fn().mockReturnValue(from(deferred));
    const component = build(getPluginDefinition);

    const install = vi.fn();
    component.install.subscribe(install);

    const pending = component.onInstallOne('c1', { version: 'v1.0.0', hash: 'sha256:abc' });
    // Closed while the fetch above is still in flight.
    component.onClose();

    resolveFetch(
      create(GetPluginDefinitionResponseSchema, {
        configSchema: [
          create(ConfigSchemaEntrySchema, { name: 'MON_COUNT', type: ConfigType.INT, defaultValue: '3' }),
        ],
      }),
    );
    await pending;

    expect(component.pendingInstall()).toBeNull();
    expect(component.schemaError()).toBe(false);
    expect(component.schemaLoading()).toBe(false);
    expect(install).not.toHaveBeenCalled();

    // The stale resolution must not have populated the cache: fetching the
    // same version again hits the catalog client a second time.
    getPluginDefinition.mockReturnValue(of(create(GetPluginDefinitionResponseSchema, { configSchema: [] })));
    await component.onInstallOne('c1', { version: 'v1.0.0', hash: 'sha256:abc' });
    expect(getPluginDefinition).toHaveBeenCalledTimes(2);
  });

  it('caches the schema per organization/plugin/version, not per version alone', async () => {
    const getPluginDefinition = vi.fn().mockReturnValue(
      of(create(GetPluginDefinitionResponseSchema, { configSchema: [] })),
    );
    const component = build(getPluginDefinition);

    const install = vi.fn();
    component.install.subscribe(install);

    await component.onInstallOne('c1', { version: 'v1.0.0', hash: 'sha256:abc' });
    expect(getPluginDefinition).toHaveBeenCalledTimes(1);

    // Same version again: served from cache, no second call.
    await component.onInstallOne('c2', { version: 'v1.0.0', hash: 'sha256:abc' });
    expect(getPluginDefinition).toHaveBeenCalledTimes(1);
  });

  it('treats configSchemaUnavailable like a fetch failure: no install, nothing cached', async () => {
    const getPluginDefinition = vi.fn().mockReturnValue(
      of(create(GetPluginDefinitionResponseSchema, { configSchema: [], configSchemaUnavailable: true })),
    );
    const component = build(getPluginDefinition);

    const install = vi.fn();
    component.install.subscribe(install);

    await component.onInstallOne('c1', { version: 'v1.0.0', hash: 'sha256:abc' });

    expect(install).not.toHaveBeenCalled();
    expect(component.pendingInstall()).toBeNull();
    expect(component.schemaError()).toBe(true);

    // Nothing cached: a retry hits the catalog client again.
    await component.onInstallOne('c1', { version: 'v1.0.0', hash: 'sha256:abc' });
    expect(getPluginDefinition).toHaveBeenCalledTimes(2);
  });

  it('drops a schema fetch superseded by a second click before it resolves', async () => {
    let resolveFirst: (value: GetPluginDefinitionResponse) => void = () => {};
    const firstDeferred = new Promise<GetPluginDefinitionResponse>((resolve) => {
      resolveFirst = resolve;
    });
    const getPluginDefinition = vi
      .fn()
      .mockReturnValueOnce(from(firstDeferred))
      .mockReturnValueOnce(of(create(GetPluginDefinitionResponseSchema, { configSchema: [] })));
    const component = build(getPluginDefinition);

    const install = vi.fn();
    component.install.subscribe(install);

    // First click, on a version whose fetch never resolves before the second.
    const firstPending = component.onInstallOne('c1', { version: 'v1.0.0', hash: 'sha256:abc' });
    // Second click, on a different row, resolves first with an empty schema.
    await component.onInstallOne('c2', { version: 'v2.0.0', hash: 'sha256:def' });

    expect(install).toHaveBeenCalledTimes(1);
    expect(install).toHaveBeenCalledWith({
      clusterIds: ['c2'],
      version: 'v2.0.0',
      hash: 'sha256:def',
      config: {},
    });

    // The stale first fetch resolving afterwards must not overwrite that.
    resolveFirst(
      create(GetPluginDefinitionResponseSchema, {
        configSchema: [
          create(ConfigSchemaEntrySchema, { name: 'MON_COUNT', type: ConfigType.INT, defaultValue: '3' }),
        ],
      }),
    );
    await firstPending;

    expect(install).toHaveBeenCalledTimes(1);
    expect(component.pendingInstall()).toBeNull();
  });
});
