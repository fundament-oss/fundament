import {
  Component,
  signal,
  computed,
  ElementRef,
  viewChild,
  inject,
  effect,
  untracked,
  OnInit,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import '@nldd/design-system/icon-button';
import '@nldd/design-system/just-in-time-education';
import '@nldd/design-system/banner';
import '@nldd/design-system/button';
import '@nldd/design-system/button-bar';
import '@nldd/design-system/modal-dialog';
import '@nldd/design-system/spacer';
import '@nldd/design-system/app-view';
import '@nldd/design-system/bar-split-view';
import '@nldd/design-system/container';
import '@nldd/design-system/toolbar';
import '@nldd/design-system/navigation-split-view';
// The split views do not pull in the panes they host, and app.html places them
// itself.
import '@nldd/design-system/split-view-pane';
import '@nldd/design-system/inline-dialog';
import '@nldd/design-system/page';
import '@nldd/design-system/simple-section';
import '@nldd/design-system/top-title-bar';
import '@nldd/design-system/cell';
import '@nldd/design-system/icon-cell';
import '@nldd/design-system/list';
import '@nldd/design-system/list-item';
import '@nldd/design-system/spacer-cell';
import '@nldd/design-system/avatar';
import '@nldd/design-system/badge';
import '@nldd/design-system/text-cell';
import '@nldd/design-system/title';
import '@nldd/design-system/tooltip';
import '@nldd/design-system/identity';
import '@nldd/design-system/menu';
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
import { inOrganization, organizationOf, withinOrganization } from './address';
import OrganizationContextService from './organization-context.service';
import type { Invitation } from '../generated/v1/invite_pb';
import ProfileComponent from './profile/profile.component';
import ApiKeysComponent from './api-keys/api-keys.component';
import NewProjectComponent from './new-project/new-project.component';
import NewClusterFormComponent from './new-cluster-form/new-cluster-form.component';
import NewNamespaceSheetComponent from './new-namespace-sheet/new-namespace-sheet.component';
import InviteMemberSheetComponent from './invite-member-sheet/invite-member-sheet.component';
import AddProjectMemberSheetComponent from './add-project-member-sheet/add-project-member-sheet.component';
import { OverlayService } from './overlay.service';
import PageNavService from './page-nav.service';
import { PRESENTATION_ENABLED } from './presentation/presentation.tokens';
import injectBrowsePluginsUrl from './plugins/browse-plugins-url';
import { CLUSTER, INVITE, ORGANIZATION } from '../connect/tokens';
import KubeClusterContextService from './plugin-resources/kube-cluster-context.service';
import MetricsHealthService from './metrics-health.service';
import PluginRegistryService from './plugin-resources/plugin-registry.service';
import PluginResourceStoreService from './plugin-resources/plugin-resource-store.service';
import opensElsewhere from './opens-elsewhere';
import { contextOf, depthForPath, isContextRoot, overviewOf, parentOf } from './nav-context';
import NavContextService from './nav-context.service';
import ContextMenuComponent from './context-menu/context-menu.component';
import ContextPickerComponent from './context-picker/context-picker.component';

const reloadApp = () => {
  window.location.reload();
};

/** Pages that stand on their own: the login page and the public platform
 *  overview. They render without the console's shell and load no organization,
 *  which would also rewrite the address into one. */
const BARE_PAGES = ['/login', '/platform'];

/** Whether a router URL is a bare page. The query does not count: another
 *  surface handing a visitor over opens the login page as `/login?app=…&path=…`. */
export function isBarePageUrl(url: string): boolean {
  return BARE_PAGES.includes(url.split(/[?#]/)[0]);
}

@Component({
  selector: 'app-root',
  imports: [
    RouterOutlet,
    DialogSyncDirective,
    OrgPickerComponent,
    ProfileComponent,
    ApiKeysComponent,
    NewProjectComponent,
    NewClusterFormComponent,
    NewNamespaceSheetComponent,
    InviteMemberSheetComponent,
    AddProjectMemberSheetComponent,
    ContextMenuComponent,
    ContextPickerComponent,
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

  private pluginRegistry = inject(PluginRegistryService);

  private pluginStore = inject(PluginResourceStoreService);

  private clusterContext = inject(KubeClusterContextService);

  private organizationClient = inject(ORGANIZATION);

  private clusterClient = inject(CLUSTER);

  private inviteClient = inject(INVITE);

  /** Whether the split view has collapsed to one visible pane. The split view
   *  measures itself, so this is not a viewport width: a narrow window can
   *  still show the menu beside the page. */
  isSingleColumn = signal(false);

  /** Whether the split view has measured itself yet. Until then
   *  `isSingleColumn` is a guess, and a narrow screen would be taken for a
   *  wide one. */
  private layoutMeasured = signal(false);

  private splitView =
    viewChild<
      ElementRef<HTMLElement & { updateComplete: Promise<unknown>; isSingleColumn: boolean }>
    >('splitView');

  /** The sheets the shell owns, not routes: navigating to them would unmount
   *  the page underneath and leave an empty pane behind the sheet. Their
   *  addresses still work; a guard opens the sheet over the home pane. */
  protected overlays = inject(OverlayService);

  protected pageNav = inject(PageNavService);

  private navContext = inject(NavContextService);

  /** The walkthrough build narrates its own path through the console, so a
   *  coach-mark pointing at the create button only competes with it. */
  private presentationEnabled = inject(PRESENTATION_ENABLED);

  /** The marketplace, where plugins are found; '' when the console's own
   *  plugins page is still the catalog (see injectBrowsePluginsUrl). */
  protected browsePluginsUrl = injectBrowsePluginsUrl();

  // Version mismatch state
  apiVersionMismatch = signal(false);

  /** The same site on the PoC; set only in a test environment, which the
   *  banner above the app then says this is. */
  protected readonly pocUrl = this.configService.getConfig().pocUrl ?? '';

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
  isBarePage = signal(isBarePageUrl(window.location.pathname));

  /** Whether this user's organizations were asked for yet. A bare page asks for
   *  none, so a visitor who signed in and then opened one gets them on the way
   *  into the console instead. */
  private organizationsRequested = false;

  currentUrl = signal(window.location.pathname);

  /**
   * How deep the stacked (narrow) view is: 0 shows the menu of the place you
   * are in, 1 the page. The split view picks the deepest pane that carries
   * `has-content` (main > sidebar), so stepping back is a matter of taking
   * content away rather than of routing.
   * On wide screens every pane shows regardless and this is inert.
   */
  stackDepth = signal(depthForPath(window.location.pathname));

  // Walkthrough (console-demo) URL; empty where the demo is not deployed, which
  // hides the header's "Take a tour" button.
  tourUrl = signal('');

  constructor() {
    // The split view only announces a change of layout, so a wide one, which
    // starts out wide, never says so. Read where its first layout pass left
    // it: that runs in a microtask after its first render, and a frame later
    // is past it.
    effect(() => {
      const element = this.splitView()?.nativeElement;
      if (!element) return;
      element.updateComplete.then(() => {
        requestAnimationFrame(() => {
          this.isSingleColumn.set(element.isSingleColumn);
          this.layoutMeasured.set(true);
        });
      });
    });

    // Moving into a place opens its overview beside the menu, rather than an
    // empty pane asking you to pick something. Only with room for both: on a
    // narrow screen the place's own address is its menu, and the back button
    // from any page leads there. Replaces the address, so Back skips it.
    effect(() => {
      if (!this.selectedOrgId() || !this.layoutMeasured() || this.isSingleColumn()) return;
      if (!this.isContextRoot()) return;
      const overview = overviewOf(contextOf(this.currentUrl()));
      untracked(() => {
        this.router.navigateByUrl(this.pageNav.path(overview), { replaceUrl: true });
      });
    });

    // The projects are part of the organization's navigation now, so they load
    // with the organization rather than when one of them is opened. A navigation
    // can select the organization before its clusters are in, so this waits for
    // those too: projects are listed per cluster.
    effect(() => {
      if (!this.selectedOrgId() || !this.organizationDataService.clustersLoaded()) return;
      untracked(() => {
        this.organizationDataService.loadProjectsAndNamespaces().catch(() => {});
      });
    });

    // Follows the project's cluster, not only the URL: opening or refreshing a
    // project URL directly gets here before the organization's projects are in,
    // so the cluster resolves later and has to bring the plugins with it. Outside
    // a project, the cluster page in view decides. Only what the shell loaded is
    // torn down: a page other than the one it loaded for resets, but on a refresh
    // it has loaded nothing, so a load a resource page started meanwhile is not
    // cut off.
    effect(() => {
      const orgId = this.selectedOrgId();
      const projectId = this.activeProjectId();
      const pageClusterId = this.activeClusterId();
      const clusterId = this.activeProjectClusterId() ?? pageClusterId;

      untracked(() => {
        let key: string | null = null;
        if (projectId) key = `${orgId}/${projectId}`;
        else if (pageClusterId) key = `${orgId}/clusters/${pageClusterId}`;

        if (key && clusterId) {
          const opened = key !== this.pluginsLoadedFor;
          this.pluginsLoadedFor = key;
          this.loadPluginsForCluster(clusterId, opened).catch(() => {});
        } else if (this.pluginsLoadedFor !== null && key !== this.pluginsLoadedFor) {
          this.pluginsLoadedFor = null;
          this.pluginRegistry.reset();
        }
      });
    });
  }

  // The organization and project or cluster page the shell last loaded plugins for.
  private pluginsLoadedFor: string | null = null;

  // Fresh only for a page just opened, so it shows plugins installed since the
  // last poll; a rerun for the same page leaves the poll's pace alone.
  private async loadPluginsForCluster(clusterId: string, fresh: boolean): Promise<void> {
    this.clusterContext.onClusterChange(clusterId);
    await this.pluginRegistry.loadPlugins(clusterId, { fresh });
  }

  async ngOnInit() {
    this.initializeTheme();
    // Only a test environment shows the status bar, so only there is it
    // fetched: the real console does not carry it in its first load. The bar
    // upgrades in place once it arrives.
    if (this.pocUrl) import('@nldd/design-system/status-bar').catch(() => {});
    this.tourUrl.set(App.tourUrlInEnglish(this.configService.getConfig().consoleDemoUrl));

    // Initialize authentication state
    await this.apiService.initializeAuth();

    // Set initial user and load organization data before child routes initialize
    const initialUser = await firstValueFrom(this.apiService.currentUser$);
    this.currentUser.set(initialUser);
    if (initialUser) {
      if (!this.isBarePage()) {
        this.organizationsRequested = true;
        await this.loadUserOrganizations();
      }
      // Only once signed in: the check is an authenticated call.
      this.metricsHealth.start();
    }

    // Subscribe to future user state changes (login/logout)
    this.apiService.currentUser$.pipe(skip(1)).subscribe((user) => {
      this.currentUser.set(user);
      if (user) {
        this.organizationsRequested = false;
        this.requestOrganizations();
        // Signing in during the session is the other way to become authenticated;
        // without this the badge stays "unknown" until the next full load.
        // start() is idempotent, so an already-running poller is left alone.
        this.metricsHealth.start();
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
        this.isBarePage.set(isBarePageUrl(event.urlAfterRedirects));
        this.requestOrganizations();
        // A sheet the shell owns stands over the page it was opened from, so
        // arriving somewhere else takes it away. One arrival is exempt: the
        // redirect that an address like /clusters/new makes to bring its own
        // sheet in.
        const openedFor = this.overlays.openedFor();
        this.overlays.openedFor.set(null);
        if (
          event.urlAfterRedirects !== openedFor &&
          event.urlAfterRedirects !== this.currentUrl()
        ) {
          this.overlays.closeAll();
        }
        this.currentUrl.set(event.urlAfterRedirects);
        this.stackDepth.set(depthForPath(event.urlAfterRedirects));
        this.updateSidebarStateFromRoute();
      });

    // The initial navigation can finish before this component exists, and a
    // guard may have redirected on the way (an overlay address lands on '/').
    // Reading the router rather than the address bar we started from keeps the
    // pane behind the sheet from rendering the URL we no longer are at.
    this.isBarePage.set(isBarePageUrl(this.router.url));
    this.currentUrl.set(this.router.url);
    this.stackDepth.set(depthForPath(this.router.url));
    // A signed-in visit to a bare page such as /login may already have been
    // redirected into the console, and that NavigationEnd went unheard.
    this.requestOrganizations();

    // Initialize sidebar state from current route
    this.updateSidebarStateFromRoute();
  }

  reloadApp = reloadApp;

  /** Loads the user's organizations once, as soon as there is a user and the
   *  page is part of the console. */
  private requestOrganizations(): void {
    if (this.organizationsRequested || !this.currentUser() || this.isBarePage()) return;
    this.organizationsRequested = true;
    this.loadUserOrganizations();
  }

  /**
   * Load the user's organizations and determine which one to select.
   * - If a valid org is stored in sessionStorage, restore it.
   * - If the user belongs to only one org, auto-select it.
   * - If the user belongs to multiple orgs, or to none, show the org picker.
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

      // Store the full list for the picker and sidebar selector
      this.organizationDataService.setUserOrganizations(orgs);

      // Determine which orgs are accepted (not pending invitation)
      const pendingOrgIds = new Set(invitations.map((i) => i.organizationId));
      const acceptedOrgs = orgs.filter((o) => !pendingOrgIds.has(o.id));

      // The address decides, so a link someone sent opens in the organization
      // it was written in rather than in the one this browser was last in. It
      // names it the way an address does, by name rather than by id.
      const wanted =
        organizationOf(this.router.url) ?? OrganizationContextService.getStoredOrganizationName();
      const addressed = wanted ? acceptedOrgs.find((o) => o.name === wanted) : undefined;

      if (addressed && invitations.length === 0) {
        await this.selectAndLoadOrganization(addressed.id);
      } else if (acceptedOrgs.length === 1 && invitations.length === 0) {
        await this.selectAndLoadOrganization(acceptedOrgs[0].id);
      } else {
        // Multiple orgs, pending invitations, or no organization at all: the
        // picker covers each. Signing in creates no organization; someone in
        // none is waiting for an operator to add them, and the picker says so.
        this.showOrgPicker.set(true);
      }
    } catch (error) {
      // eslint-disable-next-line no-console
      console.error('Failed to load organizations:', error);
    }
  }

  /**
   * A user in no organization can only wait for an operator to add them. The
   * organizations are the database's answer rather than the token's, so
   * checking again is a reload of the organizations rather than a full page
   * load; settling on one refreshes the token on the way in.
   */
  async handleOrganizationsRecheck() {
    await this.loadUserOrganizations();
  }

  /**
   * Select an organization and load its cluster data, then render child routes.
   * Projects and namespaces are loaded lazily on demand (selector open, project page visit).
   */
  private async selectAndLoadOrganization(orgId: string) {
    // Refresh the JWT so the token includes up-to-date organization
    // memberships, as selectOrganization does. The organizations come from
    // the database, the token from sign-in; an operator adding someone after
    // they signed in puts the two out of step, and every organization-scoped
    // request would be refused with the token still naming no organization.
    //
    // Every way into an organization but the sidebar switcher comes through
    // here, so this is the only refresh those paths need: a caller that
    // refreshed first would only pay for the same request twice.
    await this.apiService.refreshToken();

    const name = this.organizationNameOf(orgId);
    this.organizationContextService.setOrganizationId(orgId);
    this.organizationContextService.setOrganizationName(name);
    this.showOrgPicker.set(false);

    await this.organizationDataService.loadOrganizationData(orgId);

    // Render child routes only after cluster data is ready, so the dashboard can
    // use the pre-fetched clusterSummaries instead of making a duplicate API call.
    this.selectedOrgId.set(orgId);
    this.updateSidebarStateFromRoute();

    // An arrival that named no organization, or named one you turned out not to
    // be in: the address is written to say where you actually landed. Replacing
    // it rather than adding to it, so the back button does not lead to an
    // address that would only send you here again.
    //
    // The address is the one being arrived at, not the one already arrived on:
    // loading the organization takes a few calls, and the arrival that started
    // this can still be under way — waiting on the guard, or on the page's own
    // chunk. `router.url` then still reads as the page being left, and writing
    // from it would cancel that arrival and drop the page it named: opening
    // /clusters would land on the organization's empty root instead.
    //
    // The bare pages belong to no organization, and are left alone: a session
    // that turns up while one is open (one that already existed, or the login
    // form's own) would otherwise move it to /organizations/<name>/login, which
    // matches no page, and a hand-off back to another surface would land there
    // instead.
    const arriving = this.router.getCurrentNavigation();
    const address = arriving
      ? this.router.serializeUrl(arriving.finalUrl ?? arriving.extractedUrl)
      : this.router.url;
    if (name && !isBarePageUrl(address) && organizationOf(address) !== name) {
      this.router.navigateByUrl(inOrganization(name, address), { replaceUrl: true });
    }
  }

  /** The name an organization goes by in an address, from its id. */
  private organizationNameOf(orgId: string): string | null {
    return (
      this.organizationDataService.userOrganizations().find((org) => org.id === orgId)?.name ?? null
    );
  }

  /**
   * Handle org selection from the post-login org picker.
   */
  async handleOrgPickerSelection(orgId: string) {
    // No navigation of its own: settling on an organization writes it into the
    // address, and the rest of that address is the page you came in on.
    await this.selectAndLoadOrganization(orgId);
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
      // The newly accepted membership reaches the token on the refresh that
      // settling on the organization does.
      await this.selectAndLoadOrganization(invitation.organizationId);
      // To the front of the organization you just joined rather than to the page
      // you were reading in another one.
      this.router.navigateByUrl(inOrganization(this.organizationNameOf(invitation.organizationId)));
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

  /** The selected row says what the pane beside it holds. Collapsed to a single
   *  pane there is no such pane: the menu is a page you leave, and a highlight
   *  would point at something that is not on screen. */
  marksCurrent(): boolean {
    return !this.isSingleColumn();
  }

  /** The split view fires this whenever it collapses or expands. */
  onSingleColumnChange(event: Event): void {
    this.isSingleColumn.set((event as CustomEvent<{ singleColumn: boolean }>).detail.singleColumn);
    this.layoutMeasured.set(true);
  }

  /** Keeps `/projects/new` a real address for middle-click and "open in new
   *  tab", while a plain click opens the sheet over the page you are on. Started
   *  inside a cluster, or a project on one, the new project goes there too
   *  unless you pick another. */
  openNewProject(event: Event): void {
    if (opensElsewhere(event)) return;

    event.preventDefault();
    this.overlays.newProjectCluster.set(this.navContext.clusterId());
    this.overlays.newProject.set(true);
  }

  /**
   * An address inside the organization you are in, from one written without it.
   * The sidebar writes its links the short way — `/clusters`, `/general` — and
   * this is what turns them into the address the browser shows.
   */
  path(within = '/'): string {
    return inOrganization(this.organizationContextService.currentOrganizationName(), within);
  }

  navigateFromSidebar(event: Event, path: string): void {
    if (opensElsewhere(event)) return;

    event.preventDefault();
    this.router.navigateByUrl(this.path(path));
    // Set here too: clicking the page you are already on does not navigate, so
    // no NavigationEnd would arrive to derive the depth from.
    this.stackDepth.set(depthForPath(path));
  }

  /**
   * A back button, bubbled up from the title bar of a page: it leads to the
   * menu of the place the page is in. Only a page has one; the menu goes up a
   * level through its breadcrumb instead.
   *
   * It navigates rather than counting the stack down a step. The depth is
   * derived from the address everywhere else, so moving it by hand left the two
   * disagreeing from the first press onward: the panes emptied while the outlet
   * went on rendering the page you had just left, which reads as a button that
   * does nothing.
   */
  onPaneBack(): void {
    this.pageNav.goTo(parentOf(this.router.url));
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
    this.router.navigateByUrl(this.path(path));
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

  async selectOrganization(orgId: string) {
    // Temporarily clear selection to destroy the router outlet, so that
    // child components are recreated (and re-fetch data) after the switch.
    this.selectedOrgId.set(null);

    // Refresh the JWT so the token includes up-to-date organization memberships
    await this.apiService.refreshToken();

    // Update the organization context for API requests
    this.organizationContextService.setOrganizationId(orgId);
    this.organizationContextService.setOrganizationName(this.organizationNameOf(orgId));

    // Load the new org's cluster data
    await this.organizationDataService.loadOrganizationData(orgId);

    // Restore selection — recreates the router outlet, triggering ngOnInit in child components
    this.selectedOrgId.set(orgId);

    // The address carries the organization, so switching rewrites it. A
    // cluster or a project belongs to the one you are leaving and has no
    // counterpart here, so that address is dropped; every page of the
    // organization itself has the same one in both.
    const within = withinOrganization(this.router.url);
    const inPlace = contextOf(within).kind !== 'organization';
    this.router.navigateByUrl(
      inOrganization(this.organizationNameOf(orgId), inPlace ? '/' : within),
    );
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

  /** The project the URL is in, or null. Derived rather than stored: the
   *  menu follows where you are, it is not a mode you switch into. */
  activeProjectId = computed(() => {
    const context = contextOf(this.currentUrl());
    return context.kind === 'project' ? context.projectId : null;
  });

  /** The cluster the URL is in, or null. `clusters/new` is the create sheet
   *  over the list, not a cluster. */
  activeClusterId = computed(() => {
    const context = contextOf(this.currentUrl());
    return context.kind === 'cluster' ? context.clusterId : null;
  });

  /** The cluster the active project runs on, or null until the organization's
   *  projects have loaded. */
  activeProjectClusterId = computed(() => {
    const id = this.activeProjectId();
    return id ? (this.organizationDataService.getProjectById(id)?.cluster.id ?? null) : null;
  });

  /** Every project in the organization, for the create menu in the header. */
  organizationProjects = computed(() =>
    this.organizationDataService.organizations().flatMap((org) =>
      org.clusters.flatMap((cluster) =>
        cluster.projects.map((project) => ({
          id: project.id,
          name: project.alias || project.name,
        })),
      ),
    ),
  );

  /** Dismissed for good once you close it, so a coach-mark never becomes
   *  furniture. Stored per browser rather than per account: it is about knowing
   *  where the button is, not about anything on the server. */
  private educationDismissed = signal(
    localStorage.getItem('fundament.create-education-dismissed') === '1',
  );

  /**
   * The first cluster is the thing everything else hangs off, and the button
   * that starts one is an icon in a corner. Only with nothing to show for it
   * yet and on the organization's home: once you have gone somewhere the
   * coach-mark would point across whatever you came to read. Held back until the organization's data
   * has landed, or every account with clusters gets a flash of it while the
   * list is still empty.
   */
  showCreateEducation = computed(
    () =>
      !this.presentationEnabled &&
      !this.educationDismissed() &&
      this.organizationDataService.clustersLoaded() &&
      this.isOrganizationHome() &&
      this.organizationDataService.clusterSummaries().length === 0,
  );

  dismissCreateEducation(): void {
    localStorage.setItem('fundament.create-education-dismissed', '1');
    this.educationDismissed.set(true);
  }

  /** A place with nothing open in it: the organization's own address, a
   *  cluster's or a project's. The main pane says "pick something from the
   *  menu" there, and on a narrow screen the menu is all there is. */
  isContextRoot = computed(() => isContextRoot(this.currentUrl()));

  /** The organization's own address or its overview, which is what that
   *  address opens on a wide screen: where you are before you have gone
   *  anywhere. */
  private isOrganizationHome = computed(() => {
    const url = this.currentUrl();
    if (contextOf(url).kind !== 'organization') return false;
    return isContextRoot(url) || withinOrganization(url.split(/[?#]/)[0]) === '/general';
  });

  /** The organization the header button names. */
  currentOrganization = computed(() => {
    const orgId = this.organizationContextService.currentOrganizationId() ?? this.selectedOrgId();
    return orgId ? (this.organizationDataService.getOrganizationById(orgId) ?? null) : null;
  });
}
