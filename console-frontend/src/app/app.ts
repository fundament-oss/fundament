import { Component, signal, HostListener, inject, OnInit } from '@angular/core';
import { RouterOutlet, RouterLink, RouterLinkActive, Router } from '@angular/router';
import { CommonModule } from '@angular/common';
import { AuthnApiService } from './authn-api.service';
import type { User } from '../generated/authn/v1/authn_pb';
import { ToastService } from './toast.service';
import { versionMismatch$ } from './app.config';
import { FundamentLogoIconComponent, KubernetesIconComponent } from './icons';
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
  tablerPuzzle,
  tablerUsers,
  tablerSettings,
  tablerChartLine,
} from '@ng-icons/tabler-icons';
import { tablerCircleXFill } from '@ng-icons/tabler-icons/fill';

@Component({
  selector: 'app-root',
  imports: [
    RouterOutlet,
    RouterLink,
    RouterLinkActive,
    CommonModule,
    FundamentLogoIconComponent,
    KubernetesIconComponent,
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
      tablerPuzzle,
      tablerUsers,
      tablerSettings,
      tablerChartLine,
    }),
  ],
  templateUrl: './app.html',
})
export class App implements OnInit {
  protected readonly title = signal('fundament-console');
  private router = inject(Router);
  private apiService = inject(AuthnApiService);
  protected toastService = inject(ToastService);

  // Version mismatch state
  apiVersionMismatch = signal(false);

  // Dropdown states
  projectDropdownOpen = signal(false);
  userDropdownOpen = signal(false);
  selectedProject = signal('Project 1');
  sidebarOpen = signal(false);

  // Theme state
  isDarkMode = signal(false);

  // User state
  currentUser = signal<User | undefined>(undefined);

  async ngOnInit() {
    this.initializeTheme();

    // Initialize authentication state
    await this.apiService.initializeAuth();

    // Subscribe to user state changes
    this.apiService.currentUser$.subscribe((user) => {
      this.currentUser.set(user);
    });

    // Subscribe to API version mismatch
    versionMismatch$.subscribe((mismatch) => {
      this.apiVersionMismatch.set(mismatch);
    });
  }

  reloadApp() {
    window.location.reload();
  }

  // Check if current route is login
  isLoginPage(): boolean {
    return this.router.url === '/login';
  }

  // Check if current route is project members or permissions
  isProjectMembersOrPermissions(): boolean {
    return this.router.url === '/project-members' || this.router.url === '/project-permissions';
  }

  // Check if current route is clusters or add-cluster
  isClustersActive(): boolean {
    return this.router.url.startsWith('/clusters/') || this.router.url.startsWith('/add-cluster');
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

  @HostListener('document:click', ['$event'])
  onDocumentClick(event: Event) {
    const target = event.target as HTMLElement;
    const projectDropdown = target.closest('.project-dropdown');
    const userDropdown = target.closest('.user-dropdown');

    if (!projectDropdown) {
      this.projectDropdownOpen.set(false);
    }
    if (!userDropdown) {
      this.userDropdownOpen.set(false);
    }
  }

  toggleProjectDropdown() {
    this.projectDropdownOpen.set(!this.projectDropdownOpen());
    this.userDropdownOpen.set(false); // Close other dropdown
  }

  toggleUserDropdown() {
    this.userDropdownOpen.set(!this.userDropdownOpen());
    this.projectDropdownOpen.set(false); // Close other dropdown
  }

  async handleLogout() {
    try {
      await this.apiService.logout();
      this.router.navigate(['/login']);
    } catch (error) {
      console.error('Logout failed:', error);
    }
  }

  selectProject(project: string) {
    this.selectedProject.set(project);
    this.projectDropdownOpen.set(false);
  }

  toggleSidebar() {
    this.sidebarOpen.update((value) => !value);
  }

  closeSidebar() {
    this.sidebarOpen.set(false);
  }
}
