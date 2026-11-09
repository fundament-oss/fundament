import { Injectable, computed, inject, type WritableSignal } from '@angular/core';
import { Router } from '@angular/router';
import { inOrganization } from './address';
import NavContextService from './nav-context.service';
import OrganizationContextService from './organization-context.service';
import opensElsewhere from './opens-elsewhere';

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

  private organizationContext = inject(OrganizationContextService);

  private navContext = inject(NavContextService);

  /** Label of the back button: the menu it returns to is the place you are in,
   *  so the button names that place. */
  backText = computed(() => this.navContext.current().name);

  /**
   * An address inside the organization you are reading, from one written
   * without it: `path('/clusters')` is where the sidebar's Clusters leads. Every
   * address the console hands out goes through here, so there is one place that
   * knows an address carries its organization.
   */
  path(within = '/'): string {
    return inOrganization(this.organizationContext.currentOrganizationName(), within);
  }

  /** Plain navigation, for controls that fire their own event rather than
   *  following a link — an nldd-top-title-bar's `back`, for instance. */
  goTo(path: string): void {
    this.router.navigateByUrl(this.path(path));
  }

  /**
   * Routes client-side while leaving the element a real `<a href>`, so
   * middle-click and "open in new tab" keep working.
   */
  navigate(event: Event, path: string): void {
    // A menu item reports a plain Event; only a real click carries the modifiers
    // that mean "open this somewhere else".
    if (opensElsewhere(event)) return;

    event.preventDefault();
    this.router.navigateByUrl(this.path(path));
  }

  /**
   * Opens a sheet the shell owns instead of following the link, so it lands
   * over the page you are on rather than sending you somewhere first. The
   * element stays a real `<a href>`: middle-click and "open in new tab" go to
   * the address, which lands on the page the thing belongs to with the same
   * sheet already over it.
   */
  // eslint-disable-next-line class-methods-use-this
  openHere<T>(event: Event, sheet: WritableSignal<T>, value: T): void {
    if (opensElsewhere(event)) return;

    event.preventDefault();
    sheet.set(value);
  }
}
