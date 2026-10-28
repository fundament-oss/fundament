import { ComponentFixture, TestBed } from '@angular/core/testing';
import { ActivatedRoute, Router, convertToParamMap } from '@angular/router';
import { BehaviorSubject } from 'rxjs';
import { signal } from '@angular/core';
import { vi } from 'vitest';
import type { ParsedCrd } from '../types';
import ResourceListComponent from './resource-list.component';
import KubeClusterContextService from '../kube-cluster-context.service';
import KubePluginLoaderService from '../kube-plugin-loader.service';
import PluginRegistryService from '../plugin-registry.service';
import ProjectNamespacesService from '../project-namespaces.service';
import { TitleService } from '../../title.service';
import PageNavService from '../../page-nav.service';
import { ConfigService } from '../../config.service';

async function settle(fixture: ComponentFixture<unknown>) {
  // Each round lets an async step finish and the effect it feeds run.
  for (let i = 0; i < 4; i += 1) {
    // eslint-disable-next-line no-await-in-loop
    await fixture.whenStable();
    fixture.detectChanges();
    // eslint-disable-next-line no-await-in-loop
    await new Promise((resolve) => {
      setTimeout(resolve, 0);
    });
  }
}

interface SetupOptions {
  crdScope?: 'Namespaced' | 'Cluster';
  cluster?: boolean;
}

function setup(queryParams: Record<string, string> = {}, opts: SetupOptions = {}) {
  const crd = opts.crdScope
    ? ({
        group: 'db.io',
        version: 'v1',
        kind: 'Database',
        plural: 'databases',
        singular: 'database',
        specSchema: { properties: {} },
        additionalPrinterColumns: [],
        scope: opts.crdScope,
      } as ParsedCrd)
    : undefined;
  const loadCrdAndResources = vi.fn().mockResolvedValue({ crd, resources: [] });
  const loadResources = vi.fn().mockResolvedValue([]);
  const queryParamMap = new BehaviorSubject(convertToParamMap(queryParams));
  const navigate = vi.fn(
    (_commands: unknown, extras: { queryParams: Record<string, string | null> }) => {
      const next = { ...queryParams };
      const ns = extras.queryParams['namespace'];
      if (ns) next['namespace'] = ns;
      else delete next['namespace'];
      queryParamMap.next(convertToParamMap(next));
      return Promise.resolve(true);
    },
  );
  TestBed.configureTestingModule({
    providers: [
      {
        provide: ActivatedRoute,
        useValue: {
          snapshot: {
            data: opts.cluster ? { pluginScope: 'cluster' } : {},
            queryParamMap: queryParamMap.value,
          },
          paramMap: new BehaviorSubject(
            convertToParamMap({ id: 'p1', pluginName: 'pl', resourceKind: 'databases.db.io' }),
          ),
          queryParamMap,
        },
      },
      { provide: Router, useValue: { navigate, serializeUrl: vi.fn(), createUrlTree: vi.fn() } },
      { provide: PluginRegistryService, useValue: { getPlugin: () => undefined } },
      { provide: KubePluginLoaderService, useValue: { loadCrdAndResources, loadResources } },
      {
        provide: KubeClusterContextService,
        useValue: {
          selectedClusterId: signal('c1'),
          isLoadingClusters: signal(false),
          clusters: signal([]),
          loadClusters: vi.fn().mockResolvedValue(undefined),
        },
      },
      {
        provide: ProjectNamespacesService,
        useValue: {
          list: vi.fn().mockResolvedValue({
            names: ['tnt-p--web', 'tnt-p--db'],
            displayNames: { 'tnt-p--web': 'web', 'tnt-p--db': 'db' },
          }),
        },
      },
      { provide: TitleService, useValue: { setTitle: vi.fn() } },
      { provide: PageNavService, useValue: { backText: () => '', goTo: vi.fn(), path: vi.fn() } },
      { provide: ConfigService, useValue: { getConfig: () => ({ kubeApiProxyUrl: 'x' }) } },
    ],
  });
  const fixture = TestBed.createComponent(ResourceListComponent);
  return { fixture, loadCrdAndResources, loadResources };
}

describe('ResourceListComponent project scope', () => {
  it('lists with the project filter on a project route', async () => {
    const { fixture, loadCrdAndResources } = setup();
    fixture.detectChanges();
    await settle(fixture);
    expect(loadCrdAndResources).toHaveBeenCalledWith('pl', 'databases.db.io', 'c1', {
      projectId: 'p1',
    });
  });

  it('passes the selected namespace', async () => {
    const { fixture, loadCrdAndResources } = setup({ namespace: 'tnt-p--web' });
    fixture.detectChanges();
    await settle(fixture);
    expect(loadCrdAndResources).toHaveBeenCalledWith('pl', 'databases.db.io', 'c1', {
      projectId: 'p1',
      namespace: 'tnt-p--web',
    });
  });

  it('ignores a namespace outside the project', async () => {
    const { fixture, loadCrdAndResources } = setup({ namespace: 'other' });
    fixture.detectChanges();
    await settle(fixture);
    expect(loadCrdAndResources).toHaveBeenLastCalledWith('pl', 'databases.db.io', 'c1', {
      projectId: 'p1',
    });
  });
});

describe('ResourceListComponent namespace picker', () => {
  const select = (fixture: ComponentFixture<unknown>) =>
    (fixture.nativeElement as HTMLElement).querySelector('select');

  const choose = async (fixture: ComponentFixture<unknown>, value: string) => {
    const el = select(fixture)!;
    el.value = value;
    el.dispatchEvent(new Event('change'));
    await settle(fixture);
  };

  it('renders the project namespaces by display name', async () => {
    const { fixture } = setup({}, { crdScope: 'Namespaced' });
    fixture.detectChanges();
    await settle(fixture);
    const options = Array.from(select(fixture)!.options).map((o) => [o.value, o.text.trim()]);
    expect(options).toEqual([
      ['', 'All namespaces'],
      ['tnt-p--web', 'web'],
      ['tnt-p--db', 'db'],
    ]);
  });

  it('re-lists in the chosen namespace and keeps the select mounted', async () => {
    const { fixture, loadCrdAndResources, loadResources } = setup({}, { crdScope: 'Namespaced' });
    fixture.detectChanges();
    await settle(fixture);
    const before = select(fixture);

    await choose(fixture, 'tnt-p--web');

    expect(loadResources).toHaveBeenLastCalledWith('pl', expect.anything(), 'c1', {
      projectId: 'p1',
      namespace: 'tnt-p--web',
    });
    expect(loadCrdAndResources).toHaveBeenCalledTimes(1);
    expect(select(fixture)).toBe(before);
  });

  it('drops a superseded response', async () => {
    const { fixture, loadResources } = setup({}, { crdScope: 'Namespaced' });
    fixture.detectChanges();
    await settle(fixture);
    const item = (name: string) => ({ metadata: { name, uid: name } });
    let resolveWeb!: (v: unknown) => void;
    loadResources.mockImplementationOnce(
      () =>
        new Promise((r) => {
          resolveWeb = r;
        }),
    );
    loadResources.mockResolvedValueOnce([item('from-db')]);

    await choose(fixture, 'tnt-p--web');
    await choose(fixture, 'tnt-p--db');
    resolveWeb([item('from-web')]);
    await settle(fixture);

    const text = (fixture.nativeElement as HTMLElement).textContent;
    expect(text).toContain('from-db');
    expect(text).not.toContain('from-web');
  });

  it('is absent for a cluster-scoped kind', async () => {
    const { fixture } = setup({}, { crdScope: 'Cluster' });
    fixture.detectChanges();
    await settle(fixture);
    expect(select(fixture)).toBeNull();
  });

  it('is absent on the cluster route, which passes no scope', async () => {
    const { fixture, loadCrdAndResources } = setup({}, { crdScope: 'Namespaced', cluster: true });
    fixture.detectChanges();
    await settle(fixture);
    expect(select(fixture)).toBeNull();
    expect(loadCrdAndResources).toHaveBeenCalledWith('pl', 'databases.db.io', 'p1', undefined);
  });
});
