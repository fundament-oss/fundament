import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  computed,
  inject,
  signal,
} from '@angular/core';
import { Router, RouterLink, RouterLinkActive, RouterOutlet } from '@angular/router';
import ThemeToggleComponent from '../shared/theme-toggle';
import AuthService from '../auth.service';

// Shell wraps routes that share the nav header; task-management-technician sits outside it, since it has a different layout.
@Component({
  selector: 'app-shell',
  templateUrl: './shell.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink, RouterLinkActive, RouterOutlet, ThemeToggleComponent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: {
    '(document:click)': 'onDocumentClick($event)',
  },
})
export default class ShellComponent {
  private authService = inject(AuthService);

  private router = inject(Router);

  readonly userDropdownOpen = signal(false);

  readonly userName = computed(() => this.authService.user()?.name ?? '');

  readonly userInitials = computed(() => {
    const name = this.userName().trim();
    if (!name) return '';
    return name
      .split(/\s+/)
      .map((part) => part[0])
      .slice(0, 2)
      .join('')
      .toUpperCase();
  });

  toggleUserDropdown(): void {
    this.userDropdownOpen.update((open) => !open);
  }

  onDocumentClick(event: Event): void {
    const target = event.target as HTMLElement;
    if (!target.closest('.user-dropdown')) {
      this.userDropdownOpen.set(false);
    }
  }

  async handleLogout(): Promise<void> {
    this.userDropdownOpen.set(false);
    await this.authService.logout().catch(() => {});
    await this.router.navigate(['/login']);
  }
}
