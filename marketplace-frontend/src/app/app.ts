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
import { RouterOutlet, RouterLink, Router } from '@angular/router';
import { FundamentLogoIconComponent } from './icons';
import { ToastService } from './toast.service';
import ThemeService from './theme.service';

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

  private themeService = inject(ThemeService);

  protected toastService = inject(ToastService);

  // Theme state, owned by ThemeService so the server can render it too.
  isDarkMode = this.themeService.isDarkMode;

  // Search box value; submitting navigates to the marketplace filtered by query.
  searchQuery = signal('');

  onSearchInput(event: Event) {
    const value = (event.target as HTMLInputElement).value;
    this.searchQuery.set(value);
    // Filter in real time: reflect the query into the URL as the user types so
    // the marketplace home updates immediately. replaceUrl keeps keystrokes out
    // of the browser history, and scroll: 'manual' opts this navigation out of
    // the router's scroll-to-top so the results stay put under the reader.
    this.router.navigate(['/'], {
      queryParams: { q: value || null },
      replaceUrl: true,
      scroll: 'manual',
    });
  }

  submitSearch() {
    this.router.navigate(['/'], {
      queryParams: { q: this.searchQuery() || null },
      scroll: 'manual',
    });
  }

  toggleTheme() {
    this.themeService.toggle();
  }
}
