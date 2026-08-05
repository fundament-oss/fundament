import { Injectable, computed, inject, signal } from '@angular/core';
import { NavigationEnd, Router } from '@angular/router';
import { filter } from 'rxjs/operators';
import { OrganizationDataService } from './organization-data.service';
import OrganizationContextService from './organization-context.service';

/**
 * The bits every page needs for its `nldd-page` shell.
 *
 * On narrow screens the navigation split view stacks, drops `hide-back` on the
 * main pane, and each page's `nldd-top-title-bar` shows a back button that leads
 * up to the sidebar. Wide screens keep the sidebar in view, and the split view
 * hides that button again — so pages set `back-text` unconditionally.
 */
@Injectable({ providedIn: 'root' })
export default class PageNavService {
  private router = inject(Router);

  private organizationData = inject(OrganizationDataService);

  private organizationContext = inject(OrganizationContextService);

  private currentUrl = signal(this.router.url);

  constructor() {
    this.router.events
      .pipe(filter((event): event is NavigationEnd => event instanceof NavigationEnd))
      .subscribe((event) => this.currentUrl.set(event.urlAfterRedirects));
  }

  /** Label of the back button: the sidebar's own heading, so the button names
   *  the place it returns to. */
  backText = computed(() => {
    const projectId = this.currentUrl().match(/^\/projects\/([^/?#]+)/)?.[1];

    if (projectId) {
      const project = this.organizationData.getProjectById(projectId);
      if (project) return project.project.alias ?? project.project.name;
    }

    const orgId = this.organizationContext.currentOrganizationId();
    if (orgId) {
      const org = this.organizationData.getOrganizationById(orgId);
      if (org) return org.alias;
    }

    return 'Menu';
  });

  /** Plain navigation, for controls that fire their own event rather than
   *  following a link — an nldd-top-title-bar's `back`, for instance. */
  goTo(path: string): void {
    this.router.navigateByUrl(path);
  }

  /**
   * Routes client-side while leaving the element a real `<a href>`, so
   * middle-click and "open in new tab" keep working.
   */
  navigate(event: MouseEvent, path: string): void {
    if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0) {
      return;
    }

    event.preventDefault();
    this.router.navigateByUrl(path);
  }
}
