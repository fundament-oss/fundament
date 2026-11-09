import { Injectable, computed, inject, signal } from '@angular/core';
import { NavigationEnd, Router } from '@angular/router';
import { filter } from 'rxjs/operators';
import { OrganizationDataService } from './organization-data.service';
import { contextOf, contextRoot, type NavContext } from './nav-context';

/** The three kinds of place, as the console names and draws them. */
export const CONTEXT_KINDS = {
  organization: { label: 'Organization', icon: 'apartment-building' },
  cluster: { label: 'Cluster', icon: 'kubernetes' },
  project: { label: 'Project', icon: 'folder' },
} as const satisfies Record<NavContext['kind'], { label: string; icon: string }>;

/** A place you can be in, ready to show: what it is called and where it is. */
export interface ContextPlace {
  kind: NavContext['kind'];
  name: string;
  /** Relative to the organization. */
  path: string;
}

/**
 * The place the address is in, with the names the menu and the breadcrumb show
 * for it and the place one level up.
 *
 * Only the address decides where you are; the organization's data only fills
 * in the names, and the cluster a project runs on. Until that data is in, a
 * cluster or a project goes by its kind, and a project has no parent to return
 * to yet.
 */
@Injectable({ providedIn: 'root' })
export default class NavContextService {
  private router = inject(Router);

  private organizationData = inject(OrganizationDataService);

  /** The router's address, as a signal: `router.url` alone is not reactive. */
  readonly currentUrl = signal(this.router.url);

  constructor() {
    this.router.events
      .pipe(filter((event): event is NavigationEnd => event instanceof NavigationEnd))
      .subscribe((event) => this.currentUrl.set(event.urlAfterRedirects));
  }

  readonly context = computed(() => contextOf(this.currentUrl()));

  /** The cluster the place is on: the cluster itself, or the project's cluster
   *  once the organization's projects have loaded. */
  readonly clusterId = computed(() => {
    const context = this.context();
    if (context.kind === 'cluster') return context.clusterId;
    if (context.kind === 'project') {
      return this.organizationData.getProjectById(context.projectId)?.cluster.id ?? null;
    }
    return null;
  });

  readonly projectId = computed(() => {
    const context = this.context();
    return context.kind === 'project' ? context.projectId : null;
  });

  /** Where you are. */
  readonly current = computed<ContextPlace>(() => {
    const context = this.context();
    return { kind: context.kind, name: this.nameOf(context), path: contextRoot(context) };
  });

  /**
   * One level up, landing on the list you would have picked this place from:
   * a cluster returns to the organization's clusters, a project to its
   * cluster's projects. Null in the organization, which has nothing above it.
   */
  readonly parent = computed<ContextPlace | null>(() => {
    const context = this.context();
    switch (context.kind) {
      case 'organization':
        return null;
      case 'cluster':
        return {
          kind: 'organization',
          name: this.organizationName(),
          path: '/clusters',
        };
      case 'project': {
        const clusterId = this.clusterId();
        if (!clusterId) return null;
        return {
          kind: 'cluster',
          name: this.nameOf({ kind: 'cluster', clusterId }),
          path: `/clusters/${clusterId}/projects`,
        };
      }
      default:
        throw new Error(`unknown navigation context: ${JSON.stringify(context satisfies never)}`);
    }
  });

  private organizationName(): string {
    return (
      this.organizationData.currentOrganizationDisplayName() || CONTEXT_KINDS.organization.label
    );
  }

  private nameOf(context: NavContext): string {
    switch (context.kind) {
      case 'organization':
        return this.organizationName();
      case 'cluster':
        return (
          this.organizationData.getClusterById(context.clusterId)?.cluster.name ??
          this.organizationData.clusterSummaries().find((c) => c.id === context.clusterId)?.name ??
          CONTEXT_KINDS.cluster.label
        );
      case 'project': {
        const project = this.organizationData.getProjectById(context.projectId)?.project;
        return project?.alias || project?.name || CONTEXT_KINDS.project.label;
      }
      default:
        throw new Error(`unknown navigation context: ${JSON.stringify(context satisfies never)}`);
    }
  }
}
