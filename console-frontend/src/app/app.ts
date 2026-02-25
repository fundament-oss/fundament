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
import { filter, skip } from 'rxjs/operators';
import { NgIcon, provideIcons } from '@ng-icons/core';
import {
  tablerCircleCheck,
  tablerCircleX,
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
  tablerShieldCheck,
  tablerDatabase,
  tablerCertificate,
  tablerLock,
  tablerCloud,
} from '@ng-icons/tabler-icons';
import { firstValueFrom } from 'rxjs';
import AuthnApiService from './authn-api.service';
import type { User } from '../generated/authn/v1/authn_pb';
import { ToastService } from './toast.service';
import { versionMismatch$ } from './app.config';
import SelectorModalComponent from './selector-modal/selector-modal.component';
import OrgPickerComponent from './org-picker/org-picker.component';
import { OrganizationDataService } from './organization-data.service';
import OrganizationContextService from './organization-context.service';
import type { Invitation } from '../generated/v1/invite_pb';
import { FundamentLogoIconComponent, KubernetesIconComponent } from './icons';
import { BreadcrumbComponent, type BreadcrumbSegment } from './breadcrumb/breadcrumb.component';
import { CLUSTER, INVITE, ORGANIZATION } from '../connect/tokens';
import { fetchClusterName } from './utils/cluster-status';
import PluginNavService from './plugin-resources/plugin-nav.service';
import PluginRegistryService from './plugin-resources/plugin-registry.service';
import PluginResourceStoreService from './plugin-resources/plugin-resource-store.service';
import { kindToLabel } from './plugin-resources/crd-schema.utils';

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
    OrgPickerComponent,
    FundamentLogoIconComponent,
    KubernetesIconComponent,
    BreadcrumbComponent,
    NgIcon,
  ],
  viewProviders: [
    provideIcons({
      tablerCircleCheck,
      tablerCircleX,
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
      tablerShieldCheck,
      tablerDatabase,
      tablerCertificate,
      tablerLock,
      tablerCloud,
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

  private organizationContextService = inject(OrganizationContextService);

  protected pluginNavService = inject(PluginNavService);

  private pluginRegistry = inject(PluginRegistryService);

  private pluginStore = inject(PluginResourceStoreService);

  private organizationClient = inject(ORGANIZATION);

  private clusterClient = inject(CLUSTER);

  private inviteClient = inject(INVITE);

  private clusterNameCache = new Map<string, string>();

  // Version mismatch state
  apiVersionMismatch = signal(false);

  // Dropdown states
  userDropdownOpen = signal(false);

  sidebarOpen = signal(false);

  selectorModalOpen = signal(false);

  // Multi-org picker state (shown after login for multi-org users)
  showOrgPicker = signal(false);

  // Pending invitations for the current user
  pendingInvitations = signal<Invitation[]>([]);

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

    // Set initial user and load organization data before child routes initialize
    const initialUser = await firstValueFrom(this.apiService.currentUser$);
    this.currentUser.set(initialUser);
    if (initialUser) {
      await this.loadUserOrganizations();
    }

    // Subscribe to future user state changes (login/logout)
    this.apiService.currentUser$.pipe(skip(1)).subscribe((user) => {
      this.currentUser.set(user);
      if (user) {
        this.loadUserOrganizations();
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

  /**
   * Load the user's organizations and determine which one to select.
   * - If a valid org is stored in sessionStorage, restore it.
   * - If the user belongs to only one org, auto-select it.
   * - If the user belongs to multiple orgs, show the org picker.
   */
  private async loadUserOrganizations() {
    try {
      // Fetch organizations and pending invitations in parallel
      const [orgResponse, inviteResponse] = await Promise.all([
        firstValueFrom(this.organizationClient.listOrganizations({})),
        firstValueFrom(this.inviteClient.listInvitations({})),
      ]);

      const orgs = orgResponse.organizations;
      const invitations = inviteResponse.invitations;
      this.pendingInvitations.set(invitations);

      if (orgs.length === 0) {
        // eslint-disable-next-line no-console
        console.error('User does not belong to any organization');
        return;
      }

      // Store the full list for the picker and sidebar selector
      this.organizationDataService.setUserOrganizations(orgs);

      // Determine which orgs are accepted (not pending invitation)
      const pendingOrgIds = new Set(invitations.map((i) => i.organizationId));
      const acceptedOrgs = orgs.filter((o) => !pendingOrgIds.has(o.id));

      // Try to restore previously selected org from localStorage
      const storedOrgId = OrganizationContextService.getStoredOrganizationId();
      const storedOrgValid = storedOrgId && acceptedOrgs.some((o) => o.id === storedOrgId);

      if (storedOrgValid && invitations.length === 0) {
        await this.selectAndLoadOrganization(storedOrgId);
      } else if (acceptedOrgs.length === 1 && invitations.length === 0) {
        await this.selectAndLoadOrganization(acceptedOrgs[0].id);
      } else {
        // Multiple orgs or pending invitations: show picker
        this.showOrgPicker.set(true);
      }
    } catch (error) {
      // eslint-disable-next-line no-console
      console.error('Failed to load organizations:', error);
    }
  }

  /**
   * Select an organization and load its full data (projects, namespaces).
   */
  private async selectAndLoadOrganization(orgId: string) {
    this.organizationContextService.setOrganizationId(orgId);
    this.selectedOrgId.set(orgId);
    this.showOrgPicker.set(false);

    await this.organizationDataService.loadOrganizationData(orgId);
    this.updateSidebarStateFromRoute(this.router.url);
  }

  /**
   * Handle org selection from the post-login org picker.
   */
  async handleOrgPickerSelection(orgId: string) {
    await this.selectAndLoadOrganization(orgId);
    this.router.navigate(['/']);
  }

  /**
   * Handle accepting a pending invitation from the org picker.
   */
  async handleAcceptInvitation(invitation: Invitation) {
    try {
      await firstValueFrom(this.inviteClient.acceptInvitation({ id: invitation.id }));
      this.pendingInvitations.update((invs) => invs.filter((i) => i.id !== invitation.id));
      // Refresh the JWT so the token includes the newly accepted membership
      await this.apiService.refreshToken();
      await this.selectAndLoadOrganization(invitation.organizationId);
      this.router.navigate(['/']);
    } catch (error) {
      // eslint-disable-next-line no-console
      console.error('Failed to accept invitation:', error);
    }
  }

  /**
   * Handle declining a pending invitation from the org picker.
   */
  async handleDeclineInvitation(invitation: Invitation) {
    try {
      await firstValueFrom(this.inviteClient.declineInvitation({ id: invitation.id }));
      this.pendingInvitations.update((invs) => invs.filter((i) => i.id !== invitation.id));
      this.organizationDataService.userOrganizations.update((orgs) =>
        orgs.filter((o) => o.id !== invitation.organizationId),
      );
    } catch (error) {
      // eslint-disable-next-line no-console
      console.error('Failed to decline invitation:', error);
    }
  }

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

    // We're on an organization (or other non-project) route, so select the current org
    const currentOrgId = this.organizationContextService.currentOrganizationId();
    if (currentOrgId) {
      this.selectedOrgId.set(currentOrgId);
      this.selectedProjectId.set(null);
    }
  }

  // Update breadcrumbs based on current route data
  private async updateBreadcrumbs() {
    const configs: BreadcrumbSegment[] = [];
    let allParams: Record<string, string> = {};
    let route: ActivatedRouteSnapshot | null = this.router.routerState.snapshot.root;

    while (route) {
      allParams = { ...allParams, ...route.params };
      const bc = route.data['breadcrumbs'] as BreadcrumbSegment[] | undefined;
      if (bc) configs.push(...bc);
      route = route.firstChild ?? null;
    }

    const resolved = await Promise.all(
      configs.map((seg) => this.resolveBreadcrumb(seg, allParams)),
    );
    this.breadcrumbSegments.set(resolved);
  }

  private async resolveBreadcrumb(
    segment: BreadcrumbSegment,
    params: Record<string, string>,
  ): Promise<BreadcrumbSegment> {
    let label = segment.label;
    let route = segment.route;

    if (label === ':projectName') {
      const projectData = this.organizationDataService.getProjectById(params['id']);
      label = projectData?.project.name || 'Project';
    }

    if (label === ':pluginDisplayName') {
      const plugin = this.pluginRegistry.getPlugin(params['pluginName']);
      label = plugin?.metadata.displayName ?? params['pluginName'] ?? 'Plugin';
    }

    if (label === ':resourceKindLabel') {
      const plugin = this.pluginRegistry.getPlugin(params['pluginName']);
      const crd = plugin?.crds.find((c) => c.plural === params['resourceKind']);
      label = crd ? kindToLabel(crd.kind) : (params['resourceKind'] ?? 'Resources');
    }

    if (label === ':resourceName') {
      const plugin = this.pluginRegistry.getPlugin(params['pluginName']);
      const crd = plugin?.crds.find((c) => c.plural === params['resourceKind']);
      if (crd && params['resourceId']) {
        const resource = this.pluginStore.getResource(
          params['pluginName'],
          crd.kind,
          params['resourceId'],
        );
        label = resource?.metadata.name ?? params['resourceId'] ?? 'Resource';
      } else {
        label = params['resourceId'] ?? 'Resource';
      }
    }

    if (label === ':clusterName') {
      const clusterId = params['id'];
      if (clusterId) {
        const cached = this.clusterNameCache.get(clusterId);
        if (cached) {
          label = cached;
        } else {
          const name = await fetchClusterName(this.clusterClient, clusterId);
          if (name) {
            this.clusterNameCache.set(clusterId, name);
            label = name;
          } else {
            label = 'Cluster';
          }
        }
      } else {
        label = 'Cluster';
      }
    }

    if (route) {
      route = Object.entries(params).reduce(
        (current, [key, value]) => current.replace(`:${key}`, value),
        route,
      );
    }

    return { label, route };
  }

  // Check if current route is clusters or clusters/add
  isClustersActive(): boolean {
    return this.router.url === '/' || this.router.url.startsWith('/clusters/');
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
      this.organizationContextService.clearOrganizationId();
      this.organizationDataService.clearAll();
      this.showOrgPicker.set(false);
      this.selectedOrgId.set(null);
      this.selectedProjectId.set(null);
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
  async selectOrganization(orgId: string) {
    // Temporarily clear selection to destroy the router outlet, so that
    // child components are recreated (and re-fetch data) after the switch.
    this.selectedOrgId.set(null);
    this.selectedProjectId.set(null);

    // Refresh the JWT so the token includes up-to-date organization memberships
    await this.apiService.refreshToken();

    // Update the organization context for API requests
    this.organizationContextService.setOrganizationId(orgId);

    // Load the new org's data (projects, namespaces)
    await this.organizationDataService.loadOrganizationData(orgId);

    // Restore selection — recreates the router outlet, triggering ngOnInit in child components
    this.selectedOrgId.set(orgId);

    // Close modal
    this.selectorModalOpen.set(false);

    // Stay on org-level pages, navigate to dashboard for project routes
    const url = this.router.url;
    if (url.match(/^\/projects\/[^/]+/)) {
      this.router.navigate(['/']);
    }
  }

  selectProjectItem(projectId: string) {
    // Select project and navigate to project general page
    this.selectedProjectId.set(projectId);
    this.selectedOrgId.set(null);

    // Close modal and navigate
    this.selectorModalOpen.set(false);
    this.router.navigate(['/projects', projectId]);
  }

  /**
   * Merged list of all user orgs for the sidebar selector.
   * Includes projects only for the currently loaded org.
   */
  selectorOrganizations = computed(() => {
    const allOrgs = this.organizationDataService.userOrganizations();
    const detailedOrgs = this.organizationDataService.organizations();
    const pendingOrgIds = new Set(this.pendingInvitations().map((i) => i.organizationId));

    return allOrgs
      .filter((org) => !pendingOrgIds.has(org.id))
      .map((org) => {
        const detailed = detailedOrgs.find((d) => d.id === org.id);
        const projects = detailed ? detailed.clusters.flatMap((c) => c.projects) : [];
        return { id: org.id, name: org.name, projects };
      });
  });

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
