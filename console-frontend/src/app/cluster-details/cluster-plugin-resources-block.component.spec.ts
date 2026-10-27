import { TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import ClusterPluginResourcesBlockComponent from './cluster-plugin-resources-block.component';
import PluginNavService from '../plugin-resources/plugin-nav.service';
import PageNavService from '../page-nav.service';
import type { PluginNavGroup } from '../plugin-resources/types';

async function render(groups: PluginNavGroup[]) {
  await TestBed.configureTestingModule({
    imports: [ClusterPluginResourcesBlockComponent],
    providers: [
      { provide: PluginNavService, useValue: { clusterNav: signal(groups) } },
      {
        provide: PageNavService,
        useValue: {
          path: (within: string) => `/organizations/acme-corp${within}`,
          navigate: () => {},
        },
      },
    ],
  }).compileComponents();
  const fixture = TestBed.createComponent(ClusterPluginResourcesBlockComponent);
  fixture.componentRef.setInput('clusterId', 'c1');
  fixture.detectChanges();
  return fixture.nativeElement as HTMLElement;
}

describe('ClusterPluginResourcesBlockComponent', () => {
  it('renders one link per cluster nav item', async () => {
    const el = await render([
      {
        installationName: 'cert-manager',
        label: 'cert-manager',
        items: [
          { label: 'Cluster issuers', crdPlural: 'clusterissuers.cert-manager.io', icon: 'key' },
        ],
      },
      {
        installationName: 'ceph-rook',
        label: 'Ceph',
        items: [
          { label: 'Storage classes', crdPlural: 'storageclasses.ceph.rook.io', icon: undefined },
          { label: 'Pools', crdPlural: 'cephblockpools.ceph.rook.io', icon: undefined },
        ],
      },
    ]);

    const hrefs = Array.from(el.querySelectorAll('nldd-list-item')).map((item) =>
      item.getAttribute('href'),
    );
    expect(hrefs).toEqual([
      '/organizations/acme-corp/clusters/c1/plugin-resources/cert-manager/clusterissuers.cert-manager.io',
      '/organizations/acme-corp/clusters/c1/plugin-resources/ceph-rook/storageclasses.ceph.rook.io',
      '/organizations/acme-corp/clusters/c1/plugin-resources/ceph-rook/cephblockpools.ceph.rook.io',
    ]);
  });

  it('renders nothing when no plugin has cluster resources', async () => {
    const el = await render([]);

    expect(el.querySelector('nldd-box')).toBeNull();
  });
});
