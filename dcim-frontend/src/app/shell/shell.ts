import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  computed,
  inject,
  OnInit,
  signal,
} from '@angular/core';
import { NgTemplateOutlet } from '@angular/common';
import { NavigationEnd, Router, RouterOutlet } from '@angular/router';
import { filter } from 'rxjs/operators';
import AuthService from '../auth.service';
import ThemeService from '../theme.service';
import SecondaryNavService from './secondary-nav.service';
import OverlayService from './overlay.service';
import ProductSheetComponent from '../catalog/product-sheet/product-sheet';
import InventoryStatsService from '../inventory/inventory-stats.service';
import DatacenterHealthService from '../datacenters/datacenter-health.service';
import TaskAttentionService from '../tasks/task-attention.service';

/**
 * The sections, in the order the sidebar shows them.
 *
 * The icons say what the section holds, not what it is stored as: the catalog is
 * the reference work of what exists, the data centers are buildings, the racks
 * are racks. They used to be a stack of folders, a stack of layers and the
 * database cylinder, which all said "files" about a hall full of hardware.
 *
 * Inventory is a clipboard with a list on it: what you own, counted. Patch
 * mapping draws the cable runs themselves.
 */
const SECTIONS = [
  { text: 'Catalog', icon: 'books', path: '/catalog' },
  { text: 'Inventory', icon: 'inventory-alt', path: '/inventory' },
  { text: 'Data centers', icon: 'buildings', path: '/data-centers' },
  { text: 'Racks', icon: 'rack-servers', path: '/racks' },
  { text: 'Patch mapping', icon: 'network-patch-mapping', path: '/patch-mapping' },
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
  imports: [RouterOutlet, NgTemplateOutlet, ProductSheetComponent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
})
export default class ShellComponent implements OnInit {
  private authService = inject(AuthService);

  protected readonly theme = inject(ThemeService);

  private router = inject(Router);

  protected readonly secondaryNav = inject(SecondaryNavService);

  /** The sheets the shell owns, so a form that makes something new outlives the
   *  page it was opened from. */
  protected readonly overlays = inject(OverlayService);

  private readonly stats = inject(InventoryStatsService);

  protected readonly health = inject(DatacenterHealthService);

  protected readonly taskAttention = inject(TaskAttentionService);

  /**
   * Assets somebody has to do something about: what is broken, and what has
   * been asked for and still has to be ordered. Both are waiting on a person,
   * which is what the badge on the section is for.
   */
  protected readonly attentionCount = computed(() => {
    const s = this.stats.stats();
    return s ? s.needsRepair + s.requested : 0;
  });

  ngOnInit(): void {
    this.stats.refresh();
    this.health.refresh();
    this.taskAttention.refresh();
  }

  protected readonly sections = SECTIONS;

  /** Read from the address rather than from routerLinkActive: the links live in
   *  the list item's shadow DOM, where that directive cannot reach them. */
  private readonly currentUrl = signal(this.router.url);

  protected readonly stackDepth = signal(depthForPath(this.router.url));

  /** True while the split view shows one pane at a time. */
  private readonly singleColumn = signal(false);

  readonly userName = computed(() => this.authService.user()?.name ?? '');

  constructor() {
    this.router.events
      .pipe(filter((event): event is NavigationEnd => event instanceof NavigationEnd))
      .subscribe((event) => {
        this.currentUrl.set(event.urlAfterRedirects);
        this.stackDepth.set(depthForPath(event.urlAfterRedirects));
      });
  }

  /**
   * Whether a row in the sections list is the one you are in, and whether that
   * is worth showing. Collapsed to one column the sections list is the whole
   * screen and nothing stands beside it, so marking a row would point at a pane
   * that is not there. Same at depth 0, where the section's menu has been left
   * behind.
   */
  isCurrent(path: string): boolean {
    if (this.singleColumn() || this.stackDepth() < 1) return false;
    const current = this.currentUrl().split(/[?#]/)[0];
    return current === path || current.startsWith(`${path}/`);
  }

  /** Set by the split view when it collapses to one visible pane. */
  onSingleColumnChange(event: Event): void {
    const detail = (event as CustomEvent<{ singleColumn: boolean }>).detail;
    this.singleColumn.set(detail.singleColumn);
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
   * A back button, bubbled up from the title bar of whichever pane it sits in.
   * Which pane that is decides where you land: back from the page goes to the
   * section's menu, back from that menu to the sections. Counting down one step
   * from wherever the stack happens to be sent you to the menu you were already
   * looking at.
   */
  onPaneBack(event: Event): void {
    const pane = (event.target as HTMLElement | null)?.closest?.('nldd-split-view-pane');
    this.stackDepth.set(pane?.getAttribute('slot') === 'secondary-sidebar' ? 0 : 1);
  }

  async handleLogout(): Promise<void> {
    await this.authService.logout().catch(() => {});
    await this.router.navigate(['/login']);
  }
}
