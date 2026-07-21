import {
  Component,
  signal,
  inject,
  OnInit,
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
export default class App implements OnInit {
  private router = inject(Router);

  protected toastService = inject(ToastService);

  // Theme state
  isDarkMode = signal(false);

  // Search box value; submitting navigates to the marketplace filtered by query.
  searchQuery = signal('');

  ngOnInit() {
    this.initializeTheme();
  }

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

  // Initialize theme from an explicit saved choice, falling back to the OS
  // preference. The OS preference is never persisted here, so it keeps tracking
  // the OS on later visits until the user explicitly picks a theme.
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

  // Toggle theme in response to a user action, and persist the choice.
  toggleTheme() {
    this.isDarkMode.set(!this.isDarkMode());
    this.persistTheme();

    // Apply with view transition if supported. Use 80 ms delay to allow CSS transition on the switch to start
    setTimeout(() => {
      if (document.startViewTransition) {
        document.startViewTransition(this.applyTheme.bind(this));
      } else {
        this.applyTheme();
      }
    }, 80);
  }

  // Apply the active theme to the <html> element.
  private applyTheme() {
    const htmlElement = document.documentElement;

    if (this.isDarkMode()) {
      htmlElement.classList.add('dark');
    } else {
      htmlElement.classList.remove('dark');
    }
  }

  // Persist the user's explicit theme choice to localStorage.
  private persistTheme() {
    localStorage.setItem('theme', this.isDarkMode() ? 'dark' : 'light');
  }
}
