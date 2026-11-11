import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  computed,
  inject,
  input,
  output,
} from '@angular/core';
import '@nldd/design-system/badge';
import '@nldd/design-system/cell';
import '@nldd/design-system/icon-cell';
import '@nldd/design-system/list';
import '@nldd/design-system/list-item';
import '@nldd/design-system/page';
import '@nldd/design-system/simple-section';
import '@nldd/design-system/spacer';
import '@nldd/design-system/spacer-cell';
import '@nldd/design-system/text-cell';
import '@nldd/design-system/title';
import '@nldd/design-system/tooltip';
import { withinOrganization } from '../address';
import MetricsHealthService from '../metrics-health.service';
import NavContextService, { CONTEXT_KINDS } from '../nav-context.service';
import { OrganizationDataService } from '../organization-data.service';
import PageNavService from '../page-nav.service';
import PluginNavService from '../plugin-resources/plugin-nav.service';
import type { PluginNavGroup } from '../plugin-resources/types';
import ContextBreadcrumbComponent from './context-breadcrumb.component';

/** One row of the menu. The rows differ only in these, so the markup is one. */
export interface MenuItem {
  /** Relative to the organization. */
  path: string;
  icon: string;
  label: string;
  /** Current only on this exact address, not on the pages under it. */
  exact: boolean;
  count: number | null;
  /** Carries the warning dot while the metrics backend is unreachable. */
  metrics: boolean;
}

const item = (
  path: string,
  icon: string,
  label: string,
  extra: Partial<Pick<MenuItem, 'exact' | 'count' | 'metrics'>> = {},
): MenuItem => ({ path, icon, label, exact: false, count: null, metrics: false, ...extra });

/**
 * The menu of the place you are in. In the organization: its own settings and
 * the list of its clusters. In a cluster: the cluster's own pages, its projects,
 * and the cluster-wide resources its plugins bring. In a project: the project's
 * pages and its plugins' namespaced resources.
 *
 * A place lists the places one level down as a page (the organization's
 * Clusters, a cluster's Projects) rather than in the menu itself, and opening
 * one of them replaces this menu with that place's own. The breadcrumb on top
 * leads back up, or anywhere else through the picker.
 */
@Component({
  selector: 'app-context-menu',
  imports: [ContextBreadcrumbComponent],
  templateUrl: './context-menu.component.html',
  // The pane flex-grows what it slots, and the page inside is what has to
  // fill it: this element stands between the two and should not count.
  styles: ':host { display: contents; }',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
})
export default class ContextMenuComponent {
  protected pageNav = inject(PageNavService);

  protected metricsHealth = inject(MetricsHealthService);

  private navContext = inject(NavContextService);

  private organizationData = inject(OrganizationDataService);

  private pluginNav = inject(PluginNavService);

  /**
   * Whether the row of the page beside the menu is marked. Collapsed to a
   * single pane there is no such page in view: the menu is a screen you leave,
   * and a highlight would point at something that is not there.
   */
  readonly marksCurrent = input(true);

  readonly pickerOpen = input(false);

  readonly openPicker = output<void>();

  protected kind = computed(() => this.navContext.context().kind);

  protected kindLabel = computed(() => CONTEXT_KINDS[this.kind()].label);

  protected items = computed<MenuItem[]>(() => {
    const context = this.navContext.context();
    switch (context.kind) {
      case 'organization':
        return [
          item('/general', 'apartment-building', 'Organization overview', { exact: true }),
          item('/members', 'users', 'Members', {
            count: this.organizationData.memberCount(),
          }),
          // Only once this organization's list is in: switching keeps the
          // previous one's until then.
          item('/clusters', 'kubernetes', 'Clusters', {
            count: this.organizationData.clustersLoaded()
              ? this.organizationData.clusterSummaries().length
              : null,
          }),
          item('/plugins', 'plugin', 'Plugins'),
          item('/metrics', 'analytics', 'Metrics', { metrics: true }),
          item('/logs', 'bullet-list', 'Logs'),
          item('/quotas', 'score-meter', 'Quotas'),
        ];
      case 'cluster': {
        const base = `/clusters/${context.clusterId}`;
        const cluster = this.organizationData.getClusterById(context.clusterId)?.cluster;
        // Not before the projects are in: a 0 that turns into 3 a moment later
        // says something about the cluster that was never true.
        const projectCount =
          cluster && this.organizationData.projectsLoaded() ? cluster.projects.length : null;
        return [
          item(`${base}/general`, 'kubernetes', 'Cluster overview'),
          item(`${base}/projects`, 'folder', 'Projects', { count: projectCount }),
        ];
      }
      case 'project': {
        const base = `/projects/${context.projectId}`;
        const project = this.organizationData.getProjectById(context.projectId)?.project;
        // Roles has no count: the project summary the API returns does not
        // carry one.
        return [
          item(`${base}/general`, 'folder', 'Project overview'),
          item(`${base}/members`, 'person-2', 'Members', {
            count: project?.memberCount ?? null,
          }),
          item(`${base}/namespaces`, 'brackets-ellipsis', 'Namespaces', {
            count: project?.namespaceCount ?? null,
          }),
          item(`${base}/metrics`, 'chart-x-y-axis-line', 'Metrics', { metrics: true }),
        ];
      }
      default:
        throw new Error(`unknown navigation context: ${JSON.stringify(context satisfies never)}`);
    }
  });

  /**
   * What the cluster's plugins add, under each plugin's name. A cluster gets
   * the cluster-wide resources (an issuer for the whole cluster, a storage
   * pool), a project the namespaced ones, limited to its own namespaces.
   */
  protected pluginGroups = computed<{ group: PluginNavGroup; base: string }[]>(() => {
    const context = this.navContext.context();
    switch (context.kind) {
      case 'organization':
        return [];
      case 'cluster':
        return this.pluginNav.clusterNav().map((group) => ({
          group,
          base: `/clusters/${context.clusterId}/plugin-resources/${group.installationName}`,
        }));
      case 'project':
        return this.pluginNav.projectNav().map((group) => ({
          group,
          base: `/projects/${context.projectId}/plugin-resources/${group.installationName}`,
        }));
      default:
        throw new Error(`unknown navigation context: ${JSON.stringify(context satisfies never)}`);
    }
  });

  /** Marks a row as the page beside the menu. */
  protected isCurrent(path: string, exact: boolean): boolean {
    if (!this.marksCurrent()) return false;
    const current = withinOrganization(this.navContext.currentUrl().split(/[?#]/)[0]);
    return exact ? current === path : current === path || current.startsWith(`${path}/`);
  }
}
