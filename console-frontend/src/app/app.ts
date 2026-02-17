import {
  Component,
  signal,
  computed,
  inject,
  effect,
  untracked,
  OnInit,
  ChangeDetectionStrategy,
} from '@angular/core';
import {
  RouterOutlet,
  RouterLink,
  RouterLinkActive,
  Router,
  NavigationEnd,
  ActivatedRouteSnapshot,
} from '@angular/router';
import { filter } from 'rxjs/operators';
import { NgIcon, provideIcons } from '@ng-icons/core';
import {
  tablerCircleCheck,
  tablerAlertTriangle,
  tablerInfoCircle,
  tablerX,
  tablerMenu2,
  tablerMoon,
  tablerSun,
  tablerChevronDown,
  tablerUserCircle,
  tablerLayoutDashboard,
  tablerFolder,
  tablerFolders,
  tablerPuzzle,
  tablerUsers,
  tablerSettings,
  tablerChartLine,
  tablerChevronRight,
  tablerBuilding,
  tablerBracketsContain,
  tablerUserCog,
} from '@ng-icons/tabler-icons';
import { tablerCircleXFill } from '@ng-icons/tabler-icons/fill';
import AuthnApiService from './authn-api.service';
import type { User } from '../generated/authn/v1/authn_pb';
import { ToastService } from './toast.service';
import { versionMismatch$ } from './app.config';
import SelectorModalComponent from './selector-modal/selector-modal.component';
import { OrganizationDataService } from './organization-data.service';
import { FundamentLogoIconComponent, KubernetesIconComponent } from './icons';
import { BreadcrumbComponent, type BreadcrumbSegment } from './breadcrumb/breadcrumb.component';

const reloadApp = () => {
  window.location.reload();
};

@Component({
  selector: 'app-root',
  imports: [
    RouterOutlet,
    RouterLink,
    RouterLinkActive,
    SelectorModalComponent,
    FundamentLogoIconComponent,
    KubernetesIconComponent,
    BreadcrumbComponent,
    NgIcon,
  ],
  viewProviders: [
    provideIcons({
      tablerCircleCheck,
      tablerCircleXFill,
      tablerAlertTriangle,
      tablerInfoCircle,
      tablerX,
      tablerMenu2,
      tablerMoon,
      tablerSun,
      tablerChevronDown,
      tablerUserCircle,
      tablerLayoutDashboard,
      tablerFolder,
      tablerFolders,
      tablerPuzzle,
      tablerUsers,
      tablerSettings,
      tablerChartLine,
      tablerChevronRight,
      tablerBuilding,
      tablerBracketsContain,
      tablerUserCog,
    }),
  ],
  host: {
    '(document:click)': 'onDocumentClick($event)',
  },
  templateUrl: './app.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class App implements OnInit {
  protected readonly title = signal('fundament-console');

  private router = inject(Router);

  private apiService = inject(AuthnApiService);

  protected toastService = inject(ToastService);

  protected organizationDataService = inject(OrganizationDataService);

  // Version mismatch state
  apiVersionMismatch = signal(false);

  // Dropdown states
  userDropdownOpen = signal(false);

  sidebarOpen = signal(false);

  selectorModalOpen = signal(false);

  // Theme state
  isDarkMode = signal(false);

  // User state
  currentUser = signal<User | undefined>(undefined);

  // Nested selector state
  selectedOrgId = signal<string | null>(null);

  selectedProjectId = signal<string | null>(null);

  // Route state
  isLoginPage = signal(window.location.pathname === '/login');

  // Breadcrumb state
  breadcrumbSegments = signal<BreadcrumbSegment[]>([]);

  constructor() {
    // Refresh breadcrumbs when organization data changes (e.g. after renaming)
    effect(() => {
      this.organizationDataService.organizations();
      untracked(() => this.updateBreadcrumbs());
    });
  }

  async ngOnInit() {
    this.initializeTheme();

    // Initialize authentication state
    await this.apiService.initializeAuth();

    // Subscribe to user state changes and load organization data when user is available
    this.apiService.currentUser$.subscribe((user) => {
      this.currentUser.set(user);

      // Load organization data when user is logged in
      if (user?.organizationId) {
        this.organizationDataService.loadOrganizationData(user.organizationId).then(() => {
          // Initialize selector with the organization selected
          const orgs = this.organizationDataService.organizations();
          if (orgs.length > 0) {
            this.selectedOrgId.set(orgs[0].id);
          }
          // Re-evaluate sidebar state now that org data is available
          this.updateSidebarStateFromRoute(this.router.url);
        });
      }
    });

    // Subscribe to API version mismatch
    versionMismatch$.subscribe((mismatch) => {
      this.apiVersionMismatch.set(mismatch);
    });

    // Subscribe to route changes to update sidebar state and breadcrumbs based on current route
    this.router.events
      .pipe(filter((event): event is NavigationEnd => event instanceof NavigationEnd))
      .subscribe((event: NavigationEnd) => {
        this.isLoginPage.set(event.urlAfterRedirects === '/login');
        this.updateSidebarStateFromRoute(event.url);
        this.updateBreadcrumbs();
      });

    // Initialize sidebar state and breadcrumbs from current route
    this.updateSidebarStateFromRoute(this.router.url);
    this.updateBreadcrumbs();
  }

  reloadApp = reloadApp;

  // Update sidebar state based on current route
  private updateSidebarStateFromRoute(url: string) {
    // Match project routes: /projects/:projectId or /projects/:projectId/...
    // Exclude /projects/add which is the add-project page (not a project detail)
    const projectRouteMatch = url.match(/^\/projects\/([^/]+)/);

    if (projectRouteMatch && projectRouteMatch[1] !== 'add') {
      const projectId = projectRouteMatch[1];
      // Project route
      this.selectedProjectId.set(projectId);
      this.selectedOrgId.set(null);
      return;
    }

    // Organization routes or other routes
    // Only update if we currently have a project selected
    const hasProjectSelection = !!this.selectedProjectId();
    if (!hasProjectSelection) {
      return;
    }

    // We're on an organization (or other non-project) route, so select the organization
    const orgs = this.organizationDataService.organizations();
    if (orgs.length > 0) {
      this.selectedOrgId.set(orgs[0].id);
      this.selectedProjectId.set(null);
    }
  }

  // Update breadcrumbs based on current route data
  private updateBreadcrumbs() {
    const configs: BreadcrumbSegment[] = [];
    let allParams: Record<string, string> = {};
    let route: ActivatedRouteSnapshot | null = this.router.routerState.snapshot.root;

    while (route) {
      allParams = { ...allParams, ...route.params };
      const bc = route.data['breadcrumbs'] as BreadcrumbSegment[] | undefined;
      if (bc) configs.push(...bc);
      route = route.firstChild ?? null;
    }

    this.breadcrumbSegments.set(configs.map((seg) => this.resolveBreadcrumb(seg, allParams)));
  }

  private resolveBreadcrumb(
    segment: BreadcrumbSegment,
    params: Record<string, string>,
  ): BreadcrumbSegment {
    let label = segment.label;
    let route = segment.route;

    if (label === ':projectName') {
      const projectData = this.organizationDataService.getProjectById(params['id']);
      label = projectData?.project.name || 'Project';
    }

    if (route) {
      route = Object.entries(params).reduce(
        (current, [key, value]) => current.replace(`:${key}`, value),
        route,
      );
    }

    return { label, route };
  }

  // Check if current route is clusters or add-cluster
  isClustersActive(): boolean {
    return (
      this.router.url === '/' ||
      this.router.url.startsWith('/clusters/') ||
      this.router.url.startsWith('/add-cluster')
    );
  }

  // Check if current route is project members or roles
  isMembersActive(): boolean {
    const projectId = this.selectedProjectId();
    if (!projectId) return false;
    return (
      this.router.url.startsWith(`/projects/${projectId}/members`) ||
      this.router.url.startsWith(`/projects/${projectId}/roles`)
    );
  }

  // Initialize theme from localStorage or system preference
  private initializeTheme() {
    const savedTheme = localStorage.getItem('theme');

    if (savedTheme === 'dark' || savedTheme === 'light') {
      this.isDarkMode.set(savedTheme === 'dark');
    } else {
      // Use system preference
      this.isDarkMode.set(window.matchMedia('(prefers-color-scheme: dark)').matches);
    }

    this.applyTheme();
  }

  // Toggle theme
  toggleTheme() {
    this.isDarkMode.set(!this.isDarkMode());

    // Apply with view transition if supported. Use 80 ms delay to allow CSS transition on the switch to start
    setTimeout(() => {
      if (document.startViewTransition) {
        document.startViewTransition(this.applyTheme.bind(this));
      } else {
        this.applyTheme();
      }
    }, 80);
  }

  // Apply theme to HTML element and save to localStorage
  private applyTheme() {
    const htmlElement = document.documentElement;

    if (this.isDarkMode()) {
      htmlElement.classList.add('dark');
    } else {
      htmlElement.classList.remove('dark');
    }

    localStorage.setItem('theme', this.isDarkMode() ? 'dark' : 'light');
  }

  onDocumentClick(event: Event) {
    const target = event.target as HTMLElement;
    const userDropdown = target.closest('.user-dropdown');

    if (!userDropdown) {
      this.userDropdownOpen.set(false);
    }
  }

  toggleUserDropdown() {
    this.userDropdownOpen.set(!this.userDropdownOpen());
  }

  openSelectorModal() {
    this.selectorModalOpen.set(true);
  }

  closeSelectorModal() {
    this.selectorModalOpen.set(false);
  }

  async handleLogout() {
    try {
      await this.apiService.logout();
      this.router.navigate(['/login']);
    } catch (error) {
      // eslint-disable-next-line no-console
      console.error('Logout failed:', error);
    }
  }

  toggleSidebar() {
    this.sidebarOpen.update((value) => !value);
  }

  closeSidebar() {
    this.sidebarOpen.set(false);
  }

  // Nested selector methods
  selectOrganization(orgId: string) {
    // Select organization and navigate to clusters page
    this.selectedOrgId.set(orgId);
    this.selectedProjectId.set(null);

    // Close modal and navigate
    this.selectorModalOpen.set(false);
    this.router.navigate(['/']);
  }

  selectProjectItem(projectId: string) {
    // Select project and navigate to project general page
    this.selectedProjectId.set(projectId);
    this.selectedOrgId.set(null);

    // Close modal and navigate
    this.selectorModalOpen.set(false);
    this.router.navigate(['/projects', projectId]);
  }

  selectedType = computed<'organization' | 'project' | null>(() => {
    if (this.selectedProjectId()) return 'project';
    if (this.selectedOrgId()) return 'organization';
    return null;
  });

  settingsHeader = computed(() => {
    const type = this.selectedType();
    if (type === 'organization') return 'Organization-specific';
    if (type === 'project') return 'Project-specific';
    return '';
  });

  selectedItemDisplay = computed<{ type: 'organization' | 'project'; name: string } | null>(() => {
    const type = this.selectedType();
    if (type === 'project') {
      const projectId = this.selectedProjectId();
      if (projectId) {
        const projectData = this.organizationDataService.getProjectById(projectId);
        if (projectData) {
          return { type: 'project', name: projectData.project.name };
        }
      }
    } else if (type === 'organization') {
      const orgId = this.selectedOrgId();
      if (orgId) {
        const org = this.organizationDataService.getOrganizationById(orgId);
        if (org) {
          return { type: 'organization', name: org.name };
        }
      }
    }
    return null;
  });

  isOrganizationSelected(orgId: string): boolean {
    return this.selectedOrgId() === orgId;
  }

  isProjectSelected(projectId: string): boolean {
    return this.selectedProjectId() === projectId;
  }
}
