import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  computed,
  effect,
  inject,
  input,
  output,
  signal,
} from '@angular/core';
import '@nldd/design-system/badge';
import '@nldd/design-system/button';
import '@nldd/design-system/cell';
import '@nldd/design-system/icon-cell';
import '@nldd/design-system/list';
import '@nldd/design-system/list-item';
import '@nldd/design-system/list-item-segment';
import '@nldd/design-system/modal-dialog';
import '@nldd/design-system/search-field';
import '@nldd/design-system/spacer';
import '@nldd/design-system/spacer-cell';
import '@nldd/design-system/text';
import '@nldd/design-system/text-cell';
import '@nldd/design-system/tooltip';
import AutofocusDirective from '../autofocus.directive';
import DialogSyncDirective from '../dialog-sync.directive';
import { contextRoot } from '../nav-context';
import NavContextService from '../nav-context.service';
import { OrganizationDataService } from '../organization-data.service';
import PageNavService from '../page-nav.service';
import { ClusterStatus } from '../../generated/v1/common_pb';
import { getStatusBadgeColor, getStatusLabel } from '../utils/cluster-status';

export interface PickerProject {
  id: string;
  name: string;
  path: string;
}

export interface PickerCluster {
  id: string;
  name: string;
  path: string;
  status: ClusterStatus;
  projects: PickerProject[];
}

/**
 * The clusters and projects a search leaves standing. A cluster whose name
 * matches keeps all its projects, since you were looking for the cluster; any
 * other cluster keeps only the projects that match, and goes when none do.
 */
export function filterTree(clusters: PickerCluster[], query: string): PickerCluster[] {
  const needle = query.trim().toLowerCase();
  if (!needle) return clusters;

  const matches = (name: string) => name.toLowerCase().includes(needle);
  return clusters.flatMap((cluster) => {
    if (matches(cluster.name)) return [cluster];
    const projects = cluster.projects.filter((project) => matches(project.name));
    return projects.length > 0 ? [{ ...cluster, projects }] : [];
  });
}

/**
 * The whole organization in one tree, to move to any place in it at once rather
 * than up and down one level at a time. Opened from the breadcrumb on top of
 * the menu.
 *
 * Only the organization you are in: switching organizations refreshes your
 * token and reloads everything, and stays in the header where it was.
 */
@Component({
  selector: 'app-context-picker',
  imports: [AutofocusDirective, DialogSyncDirective],
  templateUrl: './context-picker.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
})
export default class ContextPickerComponent {
  protected pageNav = inject(PageNavService);

  private navContext = inject(NavContextService);

  private organizationData = inject(OrganizationDataService);

  readonly show = input(false);

  readonly closed = output<void>();

  protected query = signal('');

  /** Clusters folded shut by hand. Open is the default: the point of the
   *  picker is to see the whole hierarchy. */
  private collapsed = signal<ReadonlySet<string>>(new Set());

  constructor() {
    // Every time it opens, it opens fresh: last time's search would hide the
    // place you are looking for now.
    effect(() => {
      if (this.show()) {
        this.query.set('');
        this.collapsed.set(new Set());
      }
    });
  }

  protected organizationName = computed(
    () => this.organizationData.currentOrganizationDisplayName() || 'Organization',
  );

  protected organizationPath = contextRoot({ kind: 'organization' });

  private tree = computed<PickerCluster[]>(() => {
    const status = new Map(
      this.organizationData.clusterSummaries().map((cluster) => [cluster.id, cluster.status]),
    );
    return this.organizationData.organizations().flatMap((organization) =>
      organization.clusters.map((cluster) => ({
        id: cluster.id,
        name: cluster.name,
        path: contextRoot({ kind: 'cluster', clusterId: cluster.id }),
        status: status.get(cluster.id) ?? ClusterStatus.UNSPECIFIED,
        projects: cluster.projects.map((project) => ({
          id: project.id,
          name: project.alias || project.name,
          path: contextRoot({ kind: 'project', projectId: project.id }),
        })),
      })),
    );
  });

  protected clusters = computed(() => filterTree(this.tree(), this.query()));

  protected searching = computed(() => this.query().trim() !== '');

  /** The place you are in, marked in the tree so you can see where you stand. */
  protected currentPath = computed(() => this.navContext.current().path);

  /** While searching every branch is open: a match inside a closed one would
   *  be there and not be seen. */
  protected isExpanded(clusterId: string): boolean {
    return this.searching() || !this.collapsed().has(clusterId);
  }

  protected toggle(clusterId: string): void {
    this.collapsed.update((collapsed) => {
      const next = new Set(collapsed);
      if (next.has(clusterId)) next.delete(clusterId);
      else next.add(clusterId);
      return next;
    });
  }

  protected onSearch(event: Event): void {
    this.query.set((event as CustomEvent<{ value?: string }>).detail?.value ?? '');
  }

  /** Follows the row and takes the picker away. Closed here as well as by the
   *  navigation: picking the place you are already in navigates nowhere. */
  protected go(event: Event, path: string): void {
    this.pageNav.navigate(event, path);
    if (event.defaultPrevented) this.closed.emit();
  }

  protected getStatusBadgeColor = getStatusBadgeColor;

  protected getStatusLabel = getStatusLabel;
}
