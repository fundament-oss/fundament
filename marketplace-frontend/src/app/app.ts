import {
  Component,
  DestroyRef,
  afterNextRender,
  computed,
  effect,
  signal,
  inject,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import '@nldd/design-system/icon';
import '@nldd/design-system/icon-button';
import '@nldd/design-system/button';
import '@nldd/design-system/search-field';
import '@nldd/design-system/box';
import '@nldd/design-system/card';
import '@nldd/design-system/tag';
import '@nldd/design-system/sheet';
import '@nldd/design-system/page';
import '@nldd/design-system/simple-section';
import '@nldd/design-system/form-field';
import '@nldd/design-system/dropdown';
import '@nldd/design-system/multi-line-text-field';
import '@nldd/design-system/inline-dialog';
import { RouterOutlet, RouterLink, Router, ActivatedRoute } from '@angular/router';
import { NgTemplateOutlet } from '@angular/common';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { FundamentLogoIconComponent } from './icons';
import DeveloperLinkComponent from './developer-link.component';
import { ToastService } from './toast.service';
import ThemeService from './theme.service';
import { ConfigService } from './config.service';
import OrganizationContextService from './organization-context.service';
import SessionService from './session.service';
import { VARIANT } from './variant';

@Component({
  selector: 'app-root',
  imports: [
    RouterOutlet,
    RouterLink,
    NgTemplateOutlet,
    FundamentLogoIconComponent,
    DeveloperLinkComponent,
  ],
  host: {
    class: 'flex min-h-dvh flex-col',
  },
  templateUrl: './app.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
})
export default class App {
  private router = inject(Router);

  private destroyRef = inject(DestroyRef);

  private route = inject(ActivatedRoute);

  private themeService = inject(ThemeService);

  protected toastService = inject(ToastService);

  private configService = inject(ConfigService);

  // Which audience this build serves; the shell renders per-variant chrome
  // and links to the sibling deployables by URL, never by route.
  protected readonly variant = VARIANT;

  protected readonly storefrontUrl = this.configService.getConfig().storefrontUrl ?? '';

  protected readonly consoleUrl = this.configService.getConfig().consoleUrl ?? '';

  // The portal's own pages by URL; '' where no portal is deployed alongside.
  protected readonly developerUrl = this.configService.getConfig().developerUrl ?? '';

  private session = inject(SessionService);

  /**
   * Whether the storefront's visitor has a console session. 'unknown' until
   * authn-api has answered, so the header does not flash "Sign in" at someone
   * who is signed in. A build without a session surface (the demo) knows
   * straight away: nobody can be signed in there.
   */
  protected readonly sessionState = signal<'unknown' | 'signed-in' | 'signed-out'>(
    this.session.hasSessionSurface() ? 'unknown' : 'signed-out',
  );

  protected readonly user = this.session.user;

  /** What the account button says: the user's name, or a generic label for an
   *  account that has none. */
  protected readonly accountLabel = computed(() => this.user()?.name || 'Signed in');

  // The organization the developer portal acts for; the picker only shows
  // when the session belongs to more than one.
  protected organizationContext = inject(OrganizationContextService);

  // Theme state, owned by ThemeService so the server can render it too.
  isDarkMode = this.themeService.isDarkMode;

  // Search box value; submitting navigates to the marketplace filtered by query.
  searchQuery = signal('');

  constructor() {
    // Mirror ?q= back into the box so a direct load of /?q=grip, or a shared
    // link, shows the query it is filtering by instead of an empty field.
    this.route.queryParamMap.pipe(takeUntilDestroyed()).subscribe((params) => {
      this.searchQuery.set(params.get('q') ?? '');
    });

    if (this.variant === 'registry') {
      // Resolve the membership up front so the picker is filled before the
      // first registry call needs the header.
      this.organizationContext.ensureOrganizationId().catch(() => {});
    }

    // The header's menus (account, organization) only exist for a signed-in
    // visitor, so the menu element is fetched once one is needed rather than
    // weighing on every anonymous first load. Until it is defined the buttons
    // render as plain buttons; a custom element upgrades in place.
    const menuLoad = effect(() => {
      const needed =
        (this.sessionState() === 'signed-in' && !!(this.consoleUrl || this.developerUrl)) ||
        this.organizationContext.organizations().length > 1;
      if (!needed) return;
      import('@nldd/design-system/menu').catch(() => {});
      menuLoad.destroy();
    });

    if (this.variant === 'catalog' && this.session.hasSessionSurface()) {
      // In the browser only: the answer is per visitor, and a server render
      // is shared by everyone who loads that page.
      afterNextRender(() => this.watchSession());
    }
  }

  /**
   * Keeps the storefront header in step with the console session. Asked once
   * on load, and again whenever the tab comes back into view while signed out:
   * signing in happens in the console, usually in another tab, and the visitor
   * returning here should not have to reload to see it.
   */
  private watchSession() {
    const check = () => {
      this.session
        .ensureUser()
        .then((user) => this.sessionState.set(user ? 'signed-in' : 'signed-out'))
        .catch(() => this.sessionState.set('signed-out'));
    };
    const onVisible = () => {
      if (document.visibilityState === 'visible' && this.sessionState() === 'signed-out') check();
    };

    check();
    document.addEventListener('visibilitychange', onVisible);
    this.destroyRef.onDestroy(() => document.removeEventListener('visibilitychange', onVisible));
  }

  protected onOrganizationSelect(id: string) {
    if (id === this.organizationContext.organizationId()) return;
    this.organizationContext.setOrganizationId(id);
    // A detail page's plugin belongs to the previous organization; the list
    // reloads itself when the org signal changes.
    this.router.navigateByUrl('/manage').catch(() => {});
  }

  // The marketplace home is the only page whose content the ?q= param drives,
  // so it is the only page where the search box may navigate as you type.
  private isOnMarketplace() {
    return this.router.url.split(/[?#]/)[0] === '/';
  }

  onSearchInput(event: Event) {
    const value = (event.target as HTMLInputElement).value;
    this.searchQuery.set(value);
    // Live-filter only when the marketplace is already underneath the search
    // box. On /manage and /admin the keystrokes stay local until submit, so a
    // reviewer reading a submission is never navigated away mid-word.
    if (!this.isOnMarketplace()) return;
    // Reflect the query into the URL as the user types so the marketplace home
    // updates immediately. replaceUrl keeps keystrokes out of the browser
    // history, and scroll: 'manual' opts this navigation out of the router's
    // scroll-to-top so the results stay put under the reader.
    this.router.navigate(['/'], {
      queryParams: { q: value || null },
      replaceUrl: true,
      scroll: 'manual',
    });
  }

  submitSearch() {
    // Refining a query on the marketplace replaces the entry as typing does;
    // leaving another page pushes a real one so Back returns to that page.
    this.router.navigate(['/'], {
      queryParams: { q: this.searchQuery() || null },
      replaceUrl: this.isOnMarketplace(),
      scroll: 'manual',
    });
  }

  toggleTheme() {
    this.themeService.toggle();
  }
}
