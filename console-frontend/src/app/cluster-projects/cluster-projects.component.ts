import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  computed,
  effect,
  inject,
  signal,
} from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { ActivatedRoute } from '@angular/router';
import { map } from 'rxjs/operators';
import '@nldd/design-system/activity-indicator';
import '@nldd/design-system/button';
import '@nldd/design-system/icon-cell';
import '@nldd/design-system/inline-dialog';
import '@nldd/design-system/list';
import '@nldd/design-system/list-item';
import '@nldd/design-system/menu';
import '@nldd/design-system/page';
import '@nldd/design-system/rich-text';
import '@nldd/design-system/simple-section';
import '@nldd/design-system/spacer';
import '@nldd/design-system/spacer-cell';
import '@nldd/design-system/text-cell';
import '@nldd/design-system/title';
import '@nldd/design-system/toolbar';
import '@nldd/design-system/top-title-bar';
import { contextRoot } from '../nav-context';
import { OrganizationDataService } from '../organization-data.service';
import { OverlayService } from '../overlay.service';
import PageNavService from '../page-nav.service';
import { TitleService } from '../title.service';
import opensElsewhere from '../opens-elsewhere';

/** What is in a project, as the row's second line. */
function summarize(namespaceCount: number, memberCount: number): string {
  const namespaces = `${namespaceCount} namespace${namespaceCount === 1 ? '' : 's'}`;
  const members = `${memberCount} member${memberCount === 1 ? '' : 's'}`;
  return `${namespaces} · ${members}`;
}

/**
 * The projects on one cluster: the way into each of them from the cluster, the
 * way the organization's Clusters page is the way into a cluster. Opening one
 * moves the menu into that project.
 *
 * Read from what the shell has loaded for the whole organization, so the list
 * and the menu's count are the same list.
 */
@Component({
  selector: 'app-cluster-projects',
  templateUrl: './cluster-projects.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
})
export default class ClusterProjectsComponent {
  protected pageNav = inject(PageNavService);

  private organizationData = inject(OrganizationDataService);

  private overlays = inject(OverlayService);

  private titleService = inject(TitleService);

  private route = inject(ActivatedRoute);

  /** The cluster, from the route's `:id`. A signal, so moving from one
   *  cluster's projects to another's reuses the page and follows along. */
  protected id = toSignal(this.route.paramMap.pipe(map((params) => params.get('id') ?? '')), {
    initialValue: this.route.snapshot.paramMap.get('id') ?? '',
  });

  protected cluster = computed(() => this.organizationData.getClusterById(this.id())?.cluster);

  /** Until the first load has finished. Settled rather than loaded: a load
   *  where another cluster failed still has this cluster's answer. */
  protected isLoading = computed(
    () =>
      !this.organizationData.organizationLoadFailed() && !this.organizationData.projectsSettled(),
  );

  /** This cluster's list did not come back, or the organization itself did
   *  not, so its projects are unknown, not absent. */
  protected loadFailed = computed(
    () =>
      this.organizationData.organizationLoadFailed() ||
      this.organizationData.projectLoadFailures().has(this.id()),
  );

  protected retrying = signal(false);

  protected projects = computed(() =>
    (this.cluster()?.projects ?? [])
      .map((project) => ({
        id: project.id,
        name: project.alias || project.name,
        path: contextRoot({ kind: 'project', projectId: project.id }),
        summary: summarize(project.namespaceCount, project.memberCount),
      }))
      .sort((a, b) => a.name.localeCompare(b.name)),
  );

  constructor() {
    effect(() => {
      const name = this.cluster()?.name;
      this.titleService.setTitle(name ? `Projects · ${name}` : 'Projects');
    });
  }

  /** The new-project sheet, over this page. It reads the cluster from the
   *  place it is opened in, so the project goes on this one. `/projects/new`
   *  stays the address, for a new tab. */
  protected newProject(event: Event): void {
    if (opensElsewhere(event)) return;

    event.preventDefault();
    this.overlays.newProject.set(true);
  }

  protected async retry(): Promise<void> {
    this.retrying.set(true);
    try {
      // The projects hang off the organization: without it there is nothing to reload.
      if (this.organizationData.organizationLoadFailed()) {
        await this.organizationData.loadOrganizationData();
        if (this.organizationData.organizationLoadFailed()) return;
      }
      await this.organizationData.reloadProjectsAndNamespaces();
    } catch {
      // The page shows the failure again; nothing more to say here.
    } finally {
      this.retrying.set(false);
    }
  }
}
