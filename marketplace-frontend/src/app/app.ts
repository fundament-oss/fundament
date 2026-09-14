import {
  Component,
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
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { FundamentLogoIconComponent } from './icons';
import { ToastService } from './toast.service';
import ThemeService from './theme.service';
import { ConfigService } from './config.service';
import OrganizationContextService from './organization-context.service';
import { VARIANT } from './variant';

@Component({
  selector: 'app-root',
  imports: [RouterOutlet, RouterLink, FundamentLogoIconComponent],
  host: {
    class: 'flex min-h-dvh flex-col',
  },
  templateUrl: './app.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
})
export default class App {
  private router = inject(Router);

  private route = inject(ActivatedRoute);

  private themeService = inject(ThemeService);

  protected toastService = inject(ToastService);

  private configService = inject(ConfigService);

  // Which audience this build serves; the shell renders per-variant chrome
  // and links to the sibling deployables by URL, never by route.
  protected readonly variant = VARIANT;

  protected readonly developerUrl = this.configService.getConfig().developerUrl ?? '';

  protected readonly storefrontUrl = this.configService.getConfig().storefrontUrl ?? '';

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
  }

  protected onOrganizationChange(event: Event) {
    this.organizationContext.setOrganizationId((event.target as HTMLSelectElement).value);
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
