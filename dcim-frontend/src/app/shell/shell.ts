import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  computed,
  inject,
  signal,
} from '@angular/core';
import { NgTemplateOutlet } from '@angular/common';
import { NavigationEnd, Router, RouterOutlet } from '@angular/router';
import { filter } from 'rxjs/operators';
import AuthService from '../auth.service';
import ThemeService from '../theme.service';
import SecondaryNavService from './secondary-nav.service';

/** The sections, in the order the sidebar shows them. */
const SECTIONS = [
  { text: 'Catalog', icon: 'folder-on-folder', path: '/catalog' },
  { text: 'Inventory', icon: 'rectangle-stack', path: '/inventory' },
  { text: 'Data centers', icon: 'apartment-building', path: '/datacenters' },
  { text: 'Racks', icon: 'cylinder-split', path: '/racks' },
  { text: 'Patch mapping', icon: 'list', path: '/patch-mapping' },
  { text: 'Tasks', icon: 'tasks', path: '/tasks' },
];

/**
 * How deep the stacked (narrow) view is: the sections are the whole screen when
 * nothing is chosen, a section's own menu sits one step in, and what you opened
 * from it one step further.
 */
function depthForPath(url: string): number {
  const path = url.split(/[?#]/)[0];
  if (path === '/') return 0;
  return SECTIONS.some((section) => path === section.path) ? 1 : 2;
}

// Shell wraps routes that share the navigation; task-management-technician sits outside it, since it has a different layout.
@Component({
  selector: 'app-shell',
  templateUrl: './shell.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterOutlet, NgTemplateOutlet],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
})
export default class ShellComponent {
  private authService = inject(AuthService);

  protected readonly theme = inject(ThemeService);

  private router = inject(Router);

  protected readonly secondaryNav = inject(SecondaryNavService);

  protected readonly sections = SECTIONS;

  /** Read from the address rather than from routerLinkActive: the links live in
   *  the list item's shadow DOM, where that directive cannot reach them. */
  private readonly currentUrl = signal(this.router.url);

  protected readonly stackDepth = signal(depthForPath(this.router.url));

  readonly userName = computed(() => this.authService.user()?.name ?? '');

  constructor() {
    this.router.events
      .pipe(filter((event): event is NavigationEnd => event instanceof NavigationEnd))
      .subscribe((event) => {
        this.currentUrl.set(event.urlAfterRedirects);
        this.stackDepth.set(depthForPath(event.urlAfterRedirects));
      });
  }

  /** Marks the section you are in, including the pages under it. */
  isCurrent(path: string): boolean {
    const current = this.currentUrl().split(/[?#]/)[0];
    return current === path || current.startsWith(`${path}/`);
  }

  /**
   * Routes a click in-app while the row stays a real `<a href>`, so middle-click
   * and "open in new tab" keep working. Anything with a modifier is left to the
   * browser.
   */
  navigate(event: Event, path: string): void {
    if (event instanceof MouseEvent) {
      if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0) {
        return;
      }
    }

    event.preventDefault();
    this.router.navigateByUrl(path);
    // Set here too: clicking the section you are already in does not navigate,
    // so no NavigationEnd would arrive to derive the depth from.
    this.stackDepth.set(depthForPath(path));
  }

  /**
   * A page's back button, bubbled up from its title bar. Stacked (narrow) mode
   * shows the deepest pane that carries `has-content`, so dropping it reveals
   * the pane before it.
   */
  onPaneBack(): void {
    this.stackDepth.update((depth) => Math.max(depth - 1, 0));
  }

  async handleLogout(): Promise<void> {
    await this.authService.logout().catch(() => {});
    await this.router.navigate(['/login']);
  }
}
