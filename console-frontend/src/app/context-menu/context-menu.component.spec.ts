import { TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import ContextMenuComponent, { type MenuItem } from './context-menu.component';
import MetricsHealthService from '../metrics-health.service';
import NavContextService from '../nav-context.service';
import { OrganizationDataService } from '../organization-data.service';
import PageNavService from '../page-nav.service';
import PluginNavService from '../plugin-resources/plugin-nav.service';

/** The menu's rows for a context, without rendering the menu itself. */
function itemsFor(context: unknown): MenuItem[] {
  TestBed.configureTestingModule({
    providers: [
      { provide: NavContextService, useValue: { context: signal(context) } },
      {
        provide: OrganizationDataService,
        useValue: {
          memberCount: signal(null),
          clustersLoaded: signal(false),
          clusterSummaries: signal([]),
          projectsSettled: signal(false),
          projectLoadFailures: signal(new Set()),
          getClusterById: () => undefined,
          getProjectById: () => undefined,
        },
      },
      { provide: PluginNavService, useValue: {} },
      { provide: PageNavService, useValue: {} },
      { provide: MetricsHealthService, useValue: {} },
    ],
  });
  TestBed.overrideComponent(ContextMenuComponent, { set: { template: '', imports: [] } });
  const menu = TestBed.createComponent(ContextMenuComponent).componentInstance;
  return (menu as unknown as { items: () => MenuItem[] }).items();
}

describe('ContextMenuComponent', () => {
  // The per-shoot Prometheus only covers kube-system, so a project's Metrics
  // page was always empty (issue 1456).
  it('leaves Metrics out of the project menu', () => {
    const items = itemsFor({ kind: 'project', projectId: 'p-1' });
    expect(items.map((row) => row.label)).toEqual(['Project overview', 'Members', 'Namespaces']);
  });

  it('keeps Metrics in the organization menu', () => {
    const items = itemsFor({ kind: 'organization' });
    expect(items.find((row) => row.label === 'Metrics')?.path).toBe('/metrics');
  });
});
