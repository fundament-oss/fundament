import {
  Component,
  signal,
  computed,
  inject,
  effect,
  untracked,
  OnInit,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
  ViewChild,
  ElementRef,
} from '@angular/core';
import '@nldd/design-system/icon';
import '@nldd/design-system/icon-button';
import '@nldd/design-system/just-in-time-education';
import '@nldd/design-system/activity-indicator';
import '@nldd/design-system/progress-bar';
import '@nldd/design-system/banner';
import '@nldd/design-system/button-group';
import '@nldd/design-system/divider';
import '@nldd/design-system/link';
import '@nldd/design-system/box';
import '@nldd/design-system/card';
import '@nldd/design-system/collection';
import '@nldd/design-system/rich-text';
import '@nldd/design-system/button';
import '@nldd/design-system/button-bar';
import '@nldd/design-system/checkbox';
import '@nldd/design-system/checkbox-field';
import '@nldd/design-system/form';
import '@nldd/design-system/form-actions';
import '@nldd/design-system/form-field';
import '@nldd/design-system/dropdown';
import '@nldd/design-system/modal-dialog';
import '@nldd/design-system/number-field';
import '@nldd/design-system/password-field';
import '@nldd/design-system/combo-box';
import '@nldd/design-system/radio-button';
import '@nldd/design-system/radio-button-field';
import '@nldd/design-system/radio-button-group';
import '@nldd/design-system/search-field';
import '@nldd/design-system/spacer';
import '@nldd/design-system/switch-field';
import '@nldd/design-system/file-field';
import '@nldd/design-system/date-field';
import '@nldd/design-system/text-field';
import '@nldd/design-system/multi-line-text-field';
import '@nldd/design-system/token-field';
import '@nldd/design-system/toggle-button';
import '@nldd/design-system/toggle-button-group';
import '@nldd/design-system/segmented-control';
import '@nldd/design-system/app-view';
import '@nldd/design-system/bar-split-view';
import '@nldd/design-system/container';
import '@nldd/design-system/toolbar';
import '@nldd/design-system/navigation-split-view';
// The split views do not pull in the panes they host, and app.html places them
// itself.
import '@nldd/design-system/split-view-pane';
import '@nldd/design-system/inline-dialog';
import '@nldd/design-system/sheet';
import '@nldd/design-system/page';
import '@nldd/design-system/page-footer';
import '@nldd/design-system/simple-section';
import '@nldd/design-system/top-title-bar';
import '@nldd/design-system/table';
import '@nldd/design-system/cell';
import '@nldd/design-system/icon-cell';
import '@nldd/design-system/list';
import '@nldd/design-system/list-item';
import '@nldd/design-system/list-item-action';
import '@nldd/design-system/spacer-cell';
import '@nldd/design-system/timeline-track-cell';
import '@nldd/design-system/title-cell';
import '@nldd/design-system/avatar';
import '@nldd/design-system/badge';
import '@nldd/design-system/text-cell';
import '@nldd/design-system/tag';
import '@nldd/design-system/title';
import '@nldd/design-system/tooltip';
import '@nldd/design-system/identity';
import '@nldd/design-system/menu';
import '@nldd/design-system/step-indicator';
import { RouterOutlet, Router, NavigationEnd } from '@angular/router';
import { filter, skip } from 'rxjs/operators';
import { firstValueFrom } from 'rxjs';
import AuthnApiService from './authn-api.service';
import DialogSyncDirective from './dialog-sync.directive';
import type { User } from '../generated/authn/v1/authn_pb';
import { versionMismatch$ } from './app.config';
import { ConfigService } from './config.service';
import OrgPickerComponent from './org-picker/org-picker.component';
import { OrganizationDataService } from './organization-data.service';
import OrganizationContextService from './organization-context.service';
import type { Invitation } from '../generated/v1/invite_pb';
import ProfileComponent from './profile/profile.component';
import ApiKeysComponent from './api-keys/api-keys.component';
import AddProjectComponent from './add-project/add-project.component';
import AddClusterWizardLayoutComponent from './add-cluster-wizard-layout/add-cluster-wizard-layout.component';
import { OverlayService } from './overlay.service';
import PageNavService from './page-nav.service';
import { CLUSTER, INVITE, ORGANIZATION } from '../connect/tokens';
import { ClusterStatus } from '../generated/v1/common_pb';
import { getStatusBadgeColor, getStatusLabel } from './utils/cluster-status';
import KubeClusterContextService from './plugin-resources/kube-cluster-context.service';
import PluginNavService from './plugin-resources/plugin-nav.service';
import MetricsHealthService from './metrics-health.service';
import PluginRegistryService from './plugin-resources/plugin-registry.service';
import PluginResourceStoreService from './plugin-resources/plugin-resource-store.service';

const reloadApp = () => {
  window.location.reload();
};

/**
 * How deep the stacked (narrow) view is for a path: the project menu on
 * `/projects/:id`, the page on anything below or beside it. Level 0 (the
 * organization) is not a route, it is what stepping back from the menu shows.
 */
function depthForPath(url: string): number {
  const path = url.split(/[?#]/)[0];
  // Nothing chosen yet: the organization menu is the whole screen, not an empty
  // pane beside it.
  if (path === '/') return 0;
  return /^\/projects\/[^/]+$/.test(path) ? 1 : 2;
}

@Component({
  selector: 'app-root',
  imports: [
    RouterOutlet,
    DialogSyncDirective,
    OrgPickerComponent,
    ProfileComponent,
    ApiKeysComponent,
    AddProjectComponent,
    AddClusterWizardLayoutComponent,
  ],
  templateUrl: './app.html',
  // The outlet is an empty element that still counts as a flex item of
  // nldd-app-view: it took a share of the height and pushed the page it renders
  // off the top. Angular puts the component beside it, so hiding it costs
  // nothing.
  styles: 'router-outlet { display: none; }',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
})
export default class App implements OnInit {
  protected readonly title = signal('fundament-console');

  private router = inject(Router);

  /** The metrics backend, watched from outside the metrics page so the menu can
   *  say when it is unreachable. */
  protected metricsHealth = inject(MetricsHealthService);

  private apiService = inject(AuthnApiService);

  private configService = inject(ConfigService);


  protected organizationDataService = inject(OrganizationDataService);

  private organizationContextService = inject(OrganizationContextService);

  protected pluginNavService = inject(PluginNavService);

  private pluginRegistry = inject(PluginRegistryService);

  private pluginStore = inject(PluginResourceStoreService);

  private clusterContext = inject(KubeClusterContextService);

  private organizationClient = inject(ORGANIZATION);

  private clusterClient = inject(CLUSTER);

  private inviteClient = inject(INVITE);

  @ViewChild('splitView') private splitViewRef?: ElementRef<
    HTMLElement & {
      showSidebarSheet(): Promise<void>;
      hideSidebarSheet(): void;
      isSingleColumn: boolean;
    }
  >;

  /** Whether the split view has collapsed to one visible pane. The split view
   *  measures itself, so this is not a viewport width: a narrow window can
   *  still show the menu beside the page. */
  isSingleColumn = signal(false);

  /** The sheets the shell owns, not routes: navigating to them would unmount
   *  the page underneath and leave an empty pane behind the sheet. Their
   *  addresses still work; a guard opens the sheet over the home pane. */
  protected overlays = inject(OverlayService);

  protected pageNav = inject(PageNavService);

  private clusterNameCache = new Map<string, string>();

  // Version mismatch state
  apiVersionMismatch = signal(false);

  // Multi-org picker state (shown after login for multi-org users)
  showOrgPicker = signal(false);

  // Pending invitations for the current user
  pendingInvitations = signal<Invitation[]>([]);

  // Theme state. The user picks 'system', 'light' or 'dark'; 'system' follows
  // the OS and keeps following it when the OS switches.
  private readonly darkMq = window.matchMedia('(prefers-color-scheme: dark)');

  private systemPrefersDark = signal(this.darkMq.matches);

  themePreference = signal<'system' | 'light' | 'dark'>('system');

  isDarkMode = computed(() =>
    this.themePreference() === 'system'
      ? this.systemPrefersDark()
      : this.themePreference() === 'dark',
  );

  // User state
  currentUser = signal<User | undefined>(undefined);

  // Nested selector state
  selectedOrgId = signal<string | null>(null);

  // Route state
  isLoginPage = signal(window.location.pathname === '/login');

  currentUrl = signal(window.location.pathname);

  /**
   * How deep the stacked (narrow) view is: 0 shows the organization, 1 the
   * project menu, 2 the page. The split view picks the deepest pane that
   * carries `has-content` (main > secondary sidebar > primary sidebar), so
   * stepping back is a matter of taking content away rather than of routing.
   * On wide screens every pane shows regardless and this is inert.
   */
  stackDepth = signal(depthForPath(window.location.pathname));

  // Walkthrough (console-demo) URL; empty where the demo is not deployed, which
  // hides the header's "Take a tour" button.
  tourUrl = signal('');

  constructor() {
    // The projects are part of the organization's navigation now, so they load
    // with the organization rather than when one of them is opened.
    effect(() => {
      if (!this.selectedOrgId()) return;
      untracked(() => {
        this.organizationDataService.loadProjectsAndNamespaces().catch(() => {});
      });
    });

    effect(() => {
      const projectId = this.activeProjectId();

      if (projectId) {
        untracked(() => this.loadPluginsForProject(projectId));
      } else {
        untracked(() => this.pluginRegistry.reset());
      }
    });
  }

  private async loadPluginsForProject(projectId: string): Promise<void> {
    this.pluginRegistry.reset();
    await this.organizationDataService.loadProjectsAndNamespaces().catch((err) => {
      // eslint-disable-next-line no-console
      console.error('Unexpected error while loading projects and namespaces: ', err);
    });
    const projectData = this.organizationDataService.getProjectById(projectId);

    if (!projectData) return;

    const clusterId = projectData.cluster.id;
    this.clusterContext.onClusterChange(clusterId);

    await this.pluginRegistry.loadPlugins(clusterId);
  }

  async ngOnInit() {
    this.initializeTheme();
    this.tourUrl.set(App.tourUrlInEnglish(this.configService.getConfig().consoleDemoUrl));

    // Initialize authentication state
    await this.apiService.initializeAuth();

    // Set initial user and load organization data before child routes initialize
    const initialUser = await firstValueFrom(this.apiService.currentUser$);
    this.currentUser.set(initialUser);
    if (initialUser) {
      await this.loadUserOrganizations();
      // Only once signed in: the check is an authenticated call.
      this.metricsHealth.start();
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
        this.currentUrl.set(event.urlAfterRedirects);
        this.stackDepth.set(depthForPath(event.urlAfterRedirects));
        this.updateSidebarStateFromRoute();
      });

    // The initial navigation can finish before this component exists, and a
    // guard may have redirected on the way (an overlay address lands on '/').
    // Reading the router rather than the address bar we started from keeps the
    // pane behind the sheet from rendering the URL we no longer are at.
    this.isLoginPage.set(this.router.url === '/login');
    this.currentUrl.set(this.router.url);
    this.stackDepth.set(depthForPath(this.router.url));

    // Initialize sidebar state from current route
    this.updateSidebarStateFromRoute();
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
   * Select an organization and load its cluster data, then render child routes.
   * Projects and namespaces are loaded lazily on demand (selector open, project page visit).
   */
  private async selectAndLoadOrganization(orgId: string) {
    this.organizationContextService.setOrganizationId(orgId);
    this.showOrgPicker.set(false);

    await this.organizationDataService.loadOrganizationData(orgId);

    // Render child routes only after cluster data is ready, so the dashboard can
    // use the pre-fetched clusterSummaries instead of making a duplicate API call.
    this.selectedOrgId.set(orgId);
    this.updateSidebarStateFromRoute();
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
  /** The invitation the menu opened, and so the dialog that answers it. */
  invitationToAnswer = signal<Invitation | null>(null);

  openInvitation(invitation: Invitation) {
    this.invitationToAnswer.set(invitation);
  }

  async acceptInvitationFromMenu() {
    const invitation = this.invitationToAnswer();
    if (!invitation) return;
    this.invitationToAnswer.set(null);
    await this.handleAcceptInvitation(invitation);
  }

  async declineInvitationFromMenu() {
    const invitation = this.invitationToAnswer();
    if (!invitation) return;
    this.invitationToAnswer.set(null);
    await this.handleDeclineInvitation(invitation);
  }

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

  /** The organization is the only selection left; a project follows from the
   *  URL (see activeProjectId) and needs nothing stored. */
  private updateSidebarStateFromRoute() {
    const currentOrgId = this.organizationContextService.currentOrganizationId();
    if (currentOrgId && this.selectedOrgId() !== currentOrgId) {
      this.selectedOrgId.set(currentOrgId);
    }
  }

  // Update breadcrumbs based on current route data

  // Check if current route is clusters or clusters/add
  isClustersActive(): boolean {
    if (!this.marksCurrent()) return false;
    // Compare on the path alone: the router url also carries the query string.
    const path = this.currentUrl().split(/[?#]/)[0];
    return path === '/clusters' || path.startsWith('/clusters/');
  }

  /** The selected row says what the pane beside it holds. Collapsed to a single
   *  pane there is no such pane: the menu is a page you leave, and a highlight
   *  would point at something that is not on screen. */
  marksCurrent(): boolean {
    return !this.isSingleColumn();
  }

  /** The split view fires this whenever it collapses or expands, and once when
   *  it first measures itself. */
  onSingleColumnChange(event: Event): void {
    this.isSingleColumn.set((event as CustomEvent<{ singleColumn: boolean }>).detail.singleColumn);
  }

  /** Marks a sidebar item as the current page. Reads `currentUrl` so the nav
   *  re-renders on navigation; `router.url` alone is not a reactive source. */
  isNavActive(path: string, exact = false): boolean {
    if (!this.marksCurrent()) return false;
    const current = this.currentUrl().split(/[?#]/)[0];
    return exact ? current === path : current === path || current.startsWith(`${path}/`);
  }

  /** Routes a sidebar link client-side while leaving it a real `<a href>`, so
   *  middle-click and "open in new tab" keep working. */
  /** A menu item reports a plain Event; only a real click carries the modifiers
   *  that mean "open this somewhere else". */
  /** Keeps `/projects/add` a real address for middle-click and "open in new
   *  tab", while a plain click opens the sheet over the page you are on. */
  openNewProject(event: Event): void {
    if (event instanceof MouseEvent) {
      if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0) {
        return;
      }
    }

    event.preventDefault();
    this.overlays.newProject.set(true);
  }

  navigateFromSidebar(event: Event, path: string): void {
    if (event instanceof MouseEvent) {
      if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0) {
        return;
      }
    }

    event.preventDefault();
    this.router.navigateByUrl(path);
    // Picking a project lands on its menu, not on a page: /projects/:id has an
    // empty main by design, so stopping at depth 1 is what the user sees.
    // Set here too: clicking the project you are already on does not navigate,
    // so no NavigationEnd would arrive to derive the depth from.
    this.stackDepth.set(depthForPath(path));
  }

  /**
   * A page's back button, bubbled up from its nldd-top-title-bar.
   *
   * Stacked (narrow) mode shows the deepest pane that carries `has-content`, so
   * dropping it on main is what reveals the sidebar again. On wide screens the
   * split view hides the button, so this never fires there.
   */
  onPaneBack(): void {
    this.stackDepth.update((depth) => Math.max(depth - 1, 0));
  }

  isMembersActive(): boolean {
    const projectId = this.activeProjectId();
    if (!projectId) return false;
    return this.router.url.startsWith(`/projects/${projectId}/members`);
  }

  // The walkthrough resolves its narration language from `?lang`, defaulting to
  // Dutch. The console itself is English, so send visitors into the English tour.
  private static tourUrlInEnglish(consoleDemoUrl?: string): string {
    if (!consoleDemoUrl) return '';
    try {
      const url = new URL(consoleDemoUrl);
      url.searchParams.set('lang', 'en');
      return url.toString();
    } catch {
      // Not an absolute URL (misconfigured): link to it as given rather than
      // dropping the button.
      return consoleDemoUrl;
    }
  }

  // Initialize theme from an explicit saved choice, falling back to the OS
  // preference. The OS preference is never persisted here, so it keeps tracking
  // the OS on later visits until the user explicitly picks a theme.
  private initializeTheme() {
    const savedTheme = localStorage.getItem('theme');

    this.themePreference.set(
      savedTheme === 'dark' || savedTheme === 'light' ? savedTheme : 'system',
    );
    // Keep following the OS while the preference is 'system'.
    this.darkMq.addEventListener('change', (e) => {
      this.systemPrefersDark.set(e.matches);
      this.applyTheme();
    });

    this.applyTheme();
  }

  // Set theme explicitly in response to a user action, and persist the choice.
  setTheme(value: string) {
    const preference = value === 'dark' || value === 'light' ? value : 'system';
    this.themePreference.set(preference);
    this.persistTheme();
    this.applyTheme();
  }

  // Apply the active theme to the <html> element.
  private applyTheme() {
    const htmlElement = document.documentElement;

    if (this.isDarkMode()) {
      htmlElement.classList.add('dark');
    } else {
      htmlElement.classList.remove('dark');
    }

    // The design system keys its own color-scheme handling on :root[data-scheme],
    // so keep that in sync with our 'dark' class. Mirrors the inline script in index.html.
    htmlElement.dataset['scheme'] = this.isDarkMode() ? 'dark' : 'light';
  }

  // Persist the user's explicit theme choice to localStorage. 'system' clears the
  // key, so the pre-paint script in index.html falls back to the OS preference.
  private persistTheme() {
    const preference = this.themePreference();

    if (preference === 'system') {
      localStorage.removeItem('theme');
    } else {
      localStorage.setItem('theme', preference);
    }
  }

  navigateTo(path: string) {
    this.router.navigate([path]);
  }

  async handleLogout() {
    try {
      await this.apiService.logout();
      this.organizationContextService.clearOrganizationId();
      this.organizationDataService.clearAll();
      this.showOrgPicker.set(false);
      this.selectedOrgId.set(null);
      this.router.navigate(['/login']);
    } catch (error) {
      // eslint-disable-next-line no-console
      console.error('Logout failed:', error);
    }
  }

  toggleSidebar() {
    this.splitViewRef?.nativeElement.showSidebarSheet();
  }

  closeSidebar() {
    this.splitViewRef?.nativeElement.hideSidebarSheet();
  }

  async selectOrganization(orgId: string) {
    // Temporarily clear selection to destroy the router outlet, so that
    // child components are recreated (and re-fetch data) after the switch.
    this.selectedOrgId.set(null);

    // Refresh the JWT so the token includes up-to-date organization memberships
    await this.apiService.refreshToken();

    // Update the organization context for API requests
    this.organizationContextService.setOrganizationId(orgId);

    // Load the new org's cluster data
    await this.organizationDataService.loadOrganizationData(orgId);

    // Restore selection — recreates the router outlet, triggering ngOnInit in child components
    this.selectedOrgId.set(orgId);

    // Stay on org-level pages, navigate to dashboard for project routes
    const url = this.router.url;
    if (url.match(/^\/projects\/[^/]+/)) {
      this.router.navigate(['/']);
    }
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
        const projects = detailed
          ? detailed.clusters.flatMap((c) =>
              c.projects.map((p) => ({ ...p, alias: p.alias ?? p.name })),
            )
          : [];
        return { id: org.id, name: org.name, alias: org.alias, projects };
      });
  });

  /** The project sidebar, as data: the markup for each row is identical, so the
   *  rows differ only in path, icon and label. */
  /** The project the URL is in, or null. Derived rather than stored: the
   *  secondary sidebar follows where you are, it is not a mode you switch into. */
  activeProjectId = computed(() => this.currentUrl().match(/^\/projects\/([^/?#]+)/)?.[1] ?? null);

  /** Every project in the organization, for the primary sidebar. Carries the
   *  status of the cluster it runs on, so one glance over the list tells you
   *  what is up. */
  organizationProjects = computed(() => {
    const status = new Map(
      this.organizationDataService.clusterSummaries().map((c) => [c.id, c.status]),
    );

    return this.organizationDataService.organizations().flatMap((org) =>
      org.clusters.flatMap((cluster) =>
        cluster.projects.map((project) => ({
          id: project.id,
          name: project.alias || project.name,
          clusterName: cluster.name,
          namespaceCount: project.namespaceCount,
          memberCount: project.memberCount,
          status: status.get(cluster.id) ?? ClusterStatus.UNSPECIFIED,
        })),
      ),
    );
  });

  /** The project whose menu fills the secondary sidebar. */
  activeProject = computed(() => {
    const id = this.activeProjectId();
    return id ? (this.organizationProjects().find((p) => p.id === id) ?? null) : null;
  });

  getStatusBadgeColor = getStatusBadgeColor;

  getStatusLabel = getStatusLabel;

  /** The project menu. The counts ride along so the menu itself says how much
   *  is behind each item; Roles has none because the project summary the API
   *  returns does not carry one. */
  projectNavItems = computed(() => {
    const base = `/projects/${this.activeProjectId()}`;
    const project = this.activeProject();

    return [
      { path: `${base}/general`, icon: 'folder', label: 'General', exact: false, count: null },
      {
        path: `${base}/namespaces`,
        icon: 'brackets-ellipsis',
        label: 'Namespaces',
        exact: false,
        count: project?.namespaceCount ?? null,
      },
      {
        path: `${base}/metrics`,
        icon: 'chart-x-y-axis-line',
        label: 'Metrics',
        exact: false,
        count: null,
      },
      {
        path: `${base}/members`,
        icon: 'person-2',
        label: 'Members',
        exact: false,
        count: project?.memberCount ?? null,
      },
      { path: `${base}/limits`, icon: 'hand', label: 'Limits', exact: false, count: null },
    ];
  });

  pluginResourcePath(pluginName: string, crdPlural: string): string {
    return `/projects/${this.activeProjectId()}/plugin-resources/${pluginName}/${crdPlural}`;
  }

  /** The project menu route carries no page of its own, so the main pane would
   *  slot an outlet with nothing in it. */
  /** A project with nothing open under it. `/projects/add` looks the same to a
   *  pattern but is a page of its own, and swallowing it left the new-project
   *  sheet unmounted behind an empty pane. */
  /** Dismissed for good once you close it, so a coach-mark never becomes
   *  furniture. Stored per browser rather than per account: it is about knowing
   *  where the button is, not about anything on the server. */
  private educationDismissed = signal(
    localStorage.getItem('fundament.create-education-dismissed') === '1',
  );

  /**
   * The first cluster is the thing everything else hangs off, and the button
   * that starts one is an icon in a corner. Only with nothing to show for it
   * yet and nothing open: once a page is in view the coach-mark would point
   * across whatever you came to read.
   */
  showCreateEducation = computed(
    () =>
      !this.educationDismissed() &&
      this.isProjectRoot() &&
      this.organizationDataService.clusterSummaries().length === 0,
  );

  dismissCreateEducation(): void {
    localStorage.setItem('fundament.create-education-dismissed', '1');
    this.educationDismissed.set(true);
  }

  isProjectRoot = computed(() => {
    const path = this.currentUrl().split(/[?#]/)[0];
    // The app starts with nothing open, so '/' is the same empty pane.
    if (path === '/') return true;
    return path !== '/projects/add' && /^\/projects\/[^/]+$/.test(path);
  });

  /** The organization the sidebar and the header button name. The project no
   *  longer takes this over: it has its own pane. */
  currentOrganization = computed(() => {
    const orgId = this.organizationContextService.currentOrganizationId() ?? this.selectedOrgId();
    return orgId ? (this.organizationDataService.getOrganizationById(orgId) ?? null) : null;
  });

  isOrganizationSelected(orgId: string): boolean {
    return this.selectedOrgId() === orgId;
  }

  isProjectSelected(projectId: string): boolean {
    return this.activeProjectId() === projectId;
  }
}
