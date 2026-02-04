import { Component, signal, inject, OnInit, ChangeDetectionStrategy } from '@angular/core';
import { RouterOutlet, RouterLink, RouterLinkActive, Router, NavigationEnd } from '@angular/router';
import { filter } from 'rxjs/operators';
import { CommonModule } from '@angular/common';
import { AuthnApiService } from './authn-api.service';
import type { User } from '../generated/authn/v1/authn_pb';
import { ToastService } from './toast.service';
import { versionMismatch$ } from './app.config';
import { SelectorModalComponent } from './selector-modal/selector-modal.component';
import { OrganizationDataService } from './organization-data.service';
import { FundamentLogoIconComponent, KubernetesIconComponent } from './icons';
import { BreadcrumbComponent, type BreadcrumbSegment } from './breadcrumb/breadcrumb.component';
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
} from '@ng-icons/tabler-icons';
import { tablerCircleXFill } from '@ng-icons/tabler-icons/fill';

@Component({
  selector: 'app-root',
  imports: [
    RouterOutlet,
    RouterLink,
    RouterLinkActive,
    CommonModule,
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
    }),
  ],
  host: {
    '(document:click)': 'onDocumentClick($event)',
  },
  templateUrl: './app.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class App implements OnInit {
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

  // Breadcrumb state
  breadcrumbSegments = signal<BreadcrumbSegment[]>([]);

  async ngOnInit() {
    this.initializeTheme();

    // Initialize authentication state
    await this.apiService.initializeAuth();

    // Subscribe to user state changes and load organization data when user is available
    this.apiService.currentUser$.subscribe((user) => {
      this.currentUser.set(user);

      // Load organization data when user is logged in
      if (user?.organizationId) {
        this.organizationDataService.loadOrganizationData().then(() => {
          // Initialize selector with the organization selected
          const orgs = this.organizationDataService.organizations();
          if (orgs.length > 0) {
            this.selectedOrgId.set(orgs[0].id);
          }
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
        this.updateSidebarStateFromRoute(event.url);
        this.updateBreadcrumbs(event.url);
      });

    // Initialize sidebar state and breadcrumbs from current route
    this.updateSidebarStateFromRoute(this.router.url);
    this.updateBreadcrumbs(this.router.url);
  }

  reloadApp() {
    window.location.reload();
  }

  // Update sidebar state based on current route
  private updateSidebarStateFromRoute(url: string) {
    // Match project routes: /projects/:projectId or /projects/:projectId/...
    const projectRouteMatch = url.match(/^\/projects\/([^/]+)/);

    if (projectRouteMatch) {
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

  // Update breadcrumbs based on current route
  private updateBreadcrumbs(url: string) {
    const segments: BreadcrumbSegment[] = [];

    // Home/Dashboard - Clusters
    if (url === '/') {
      segments.push({ label: 'Clusters' });
    }
    // Projects list
    else if (url === '/projects') {
      segments.push({ label: 'Projects', route: '/projects' });
    }
    // Add project
    else if (url === '/projects/add') {
      segments.push({ label: 'Projects', route: '/projects' });
      segments.push({ label: 'Add project' });
    }
    // Project routes
    else if (url.startsWith('/projects/')) {
      const projectMatch = url.match(/^\/projects\/([^/]+)(\/(.+))?$/);
      if (projectMatch) {
        const projectId = projectMatch[1];
        const subRoute = projectMatch[3];

        // Get project name
        const projectData = this.organizationDataService.getProjectById(projectId);
        const projectName = projectData?.project.name || 'Project';

        segments.push({ label: projectName, route: `/projects/${projectId}` });

        // Add sub-route segment
        if (subRoute === 'namespaces') {
          segments.push({ label: 'Namespaces' });
        } else if (subRoute === 'members') {
          segments.push({ label: 'Members' });
        } else if (subRoute === 'usage') {
          segments.push({ label: 'Usage' });
        } else if (subRoute === 'settings') {
          segments.push({ label: 'Settings' });
        } else if (!subRoute) {
          segments.push({ label: 'General' });
        }
      }
    }
    // Cluster detail routes
    else if (url.startsWith('/clusters/')) {
      const clusterMatch = url.match(/^\/clusters\/([^/]+)(\/(.+))?$/);
      if (clusterMatch) {
        const subRoute = clusterMatch[3];

        segments.push({ label: 'Clusters', route: '/' });
        segments.push({ label: 'Cluster details' });

        // Add sub-route segment
        if (subRoute === 'nodes') {
          segments.push({ label: 'Nodes' });
        } else if (subRoute === 'plugins') {
          segments.push({ label: 'Plugins' });
        }
      }
    }
    // Add cluster wizard
    else if (url.startsWith('/add-cluster')) {
      segments.push({ label: 'Clusters', route: '/' });
      segments.push({ label: 'Add cluster' });
    }
    // Plugins list
    else if (url === '/plugins') {
      segments.push({ label: 'Plugins' });
    }
    // Plugin details
    else if (url.startsWith('/plugins/')) {
      const pluginMatch = url.match(/^\/plugins\/([^/]+)$/);
      if (pluginMatch) {
        segments.push({ label: 'Plugins', route: '/plugins' });
        // We could fetch plugin name here, but for now just show "Plugin details"
        segments.push({ label: 'Plugin details' });
      }
    }
    // Organization settings
    else if (url === '/organization') {
      segments.push({ label: 'Organization' });
    }
    // Organization members
    else if (url === '/organization/members') {
      segments.push({ label: 'Organization members' });
    }
    // Usage
    else if (url === '/usage') {
      segments.push({ label: 'Usage' });
    }
    // Profile
    else if (url === '/profile') {
      segments.push({ label: 'Profile' });
    }
    // API Keys
    else if (url === '/api-keys') {
      segments.push({ label: 'API keys' });
    }

    this.breadcrumbSegments.set(segments);
  }

  // Check if current route is login
  isLoginPage(): boolean {
    return this.router.url === '/login';
  }

  // Check if current route is project members or permissions
  isProjectMembersOrPermissions(): boolean {
    return this.router.url.includes('/members');
  }

  // Check if current route is clusters or add-cluster
  isClustersActive(): boolean {
    return (
      this.router.url === '/' ||
      this.router.url.startsWith('/clusters/') ||
      this.router.url.startsWith('/add-cluster')
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

  getSelectedType(): 'organization' | 'project' | null {
    if (this.selectedProjectId()) return 'project';
    if (this.selectedOrgId()) return 'organization';
    return null;
  }

  getSettingsHeader(): string {
    const type = this.getSelectedType();
    if (type === 'organization') return 'Organization-specific';
    if (type === 'project') return 'Project-specific';
    return '';
  }

  isOrganizationSelected(orgId: string): boolean {
    return this.selectedOrgId() === orgId;
  }

  isProjectSelected(projectId: string): boolean {
    return this.selectedProjectId() === projectId;
  }

  // Cached values to avoid recomputing the selected item display on every change detection run.
  private cachedSelectedDisplay: {
    type: 'organization' | 'project';
    name: string;
  } | null = null;

  private cachedSelectedType: 'organization' | 'project' | null = null;
  private cachedOrgId: string | null = null;
  private cachedProjectId: string | null = null;

  getSelectedItemDisplay(): {
    type: 'organization' | 'project';
    name: string;
  } | null {
    const selectedType = this.getSelectedType();

    if (!selectedType) {
      this.cachedSelectedDisplay = null;
      this.cachedSelectedType = null;
      this.cachedOrgId = null;
      this.cachedProjectId = null;
      return null;
    }

    const currentOrgId = this.selectedOrgId();
    const currentProjectId = this.selectedProjectId();

    // Return cached value if the selection (type and IDs) hasn't changed.
    if (
      this.cachedSelectedType === selectedType &&
      this.cachedOrgId === currentOrgId &&
      this.cachedProjectId === currentProjectId
    ) {
      return this.cachedSelectedDisplay;
    }

    let result: {
      type: 'organization' | 'project';
      name: string;
    } | null = null;

    if (selectedType === 'project') {
      const projectId = currentProjectId;
      if (projectId) {
        const projectData = this.organizationDataService.getProjectById(projectId);
        if (projectData) {
          result = { type: 'project', name: projectData.project.name };
        }
      }
    } else if (selectedType === 'organization') {
      const orgId = currentOrgId;
      if (orgId) {
        const org = this.organizationDataService.getOrganizationById(orgId);
        if (org) {
          result = { type: 'organization', name: org.name };
        }
      }
    }

    // Update cache before returning.
    this.cachedSelectedDisplay = result;
    this.cachedSelectedType = selectedType;
    this.cachedOrgId = currentOrgId;
    this.cachedProjectId = currentProjectId;

    return result;
  }
}
