import { Component, signal, HostListener, inject, OnInit } from '@angular/core';
import { RouterOutlet, RouterLink, RouterLinkActive, Router } from '@angular/router';
import { CommonModule } from '@angular/common';
import { AuthnApiService } from './authn-api.service';
import type { User } from '../generated/authn/v1/authn_pb';
import { ToastService } from './toast.service';
import { versionMismatch$ } from './app.config';
import { SelectorModalComponent } from './selector-modal/selector-modal.component';
import { OrganizationDataService } from './organization-data.service';
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
  tablerFolders,
  tablerPuzzle,
  tablerUsers,
  tablerSettings,
  tablerChartLine,
  tablerChevronRight,
  tablerBracketsContain,
  tablerBuilding,
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
      tablerBracketsContain,
      tablerBuilding,
    }),
  ],
  templateUrl: './app.html',
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
  selectedNamespaceId = signal<string | null>(null);

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
    return this.router.url.includes('/members') || this.router.url === '/project-permissions';
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
    this.selectedNamespaceId.set(null);

    // Close modal and navigate
    this.selectorModalOpen.set(false);
    this.router.navigate(['/']);
  }

  selectProjectItem(projectId: string) {
    // Select project and navigate to namespaces page
    this.selectedProjectId.set(projectId);
    this.selectedOrgId.set(null);
    this.selectedNamespaceId.set(null);

    // Close modal and navigate
    this.selectorModalOpen.set(false);
    this.router.navigate(['/projects', projectId, 'namespaces']);
  }

  selectNamespaceItem(namespaceId: string) {
    this.selectedNamespaceId.set(namespaceId);
    this.selectedOrgId.set(null);

    // Find the project that contains this namespace
    let projectId: string | null = null;
    for (const org of this.organizationDataService.organizations()) {
      for (const project of org.projects) {
        if (project.namespaces.some((ns) => ns.id === namespaceId)) {
          projectId = project.id;
          break;
        }
      }
      if (projectId) break;
    }

    this.selectedProjectId.set(projectId);

    // Close modal and navigate
    this.selectorModalOpen.set(false);
    if (projectId) {
      this.router.navigate(['/projects', projectId, 'namespaces', namespaceId, 'members']);
    }
  }

  getSelectedType(): 'organization' | 'project' | 'namespace' | null {
    if (this.selectedNamespaceId()) return 'namespace';
    if (this.selectedProjectId()) return 'project';
    if (this.selectedOrgId()) return 'organization';
    return null;
  }

  getSettingsHeader(): string {
    const type = this.getSelectedType();
    if (type === 'organization') return 'Organization-specific';
    if (type === 'project') return 'Project-specific';
    if (type === 'namespace') return 'Namespace-specific';
    return '';
  }

  isOrganizationSelected(orgId: string): boolean {
    return this.selectedOrgId() === orgId;
  }

  isProjectSelected(projectId: string): boolean {
    return this.selectedProjectId() === projectId;
  }

  isNamespaceSelected(namespaceId: string): boolean {
    return this.selectedNamespaceId() === namespaceId;
  }

  getSelectedItemDisplay(): {
    type: 'organization' | 'project' | 'namespace';
    name: string;
  } | null {
    const selectedType = this.getSelectedType();

    if (!selectedType) return null;

    if (selectedType === 'namespace') {
      const namespaceId = this.selectedNamespaceId();
      for (const org of this.organizationDataService.organizations()) {
        for (const project of org.projects) {
          const namespace = project.namespaces.find((ns) => ns.id === namespaceId);
          if (namespace) {
            return { type: 'namespace', name: namespace.name };
          }
        }
      }
    } else if (selectedType === 'project') {
      const projectId = this.selectedProjectId();
      for (const org of this.organizationDataService.organizations()) {
        const project = org.projects.find((p) => p.id === projectId);
        if (project) {
          return { type: 'project', name: project.name };
        }
      }
    } else if (selectedType === 'organization') {
      const orgId = this.selectedOrgId();
      const org = this.organizationDataService.organizations().find((o) => o.id === orgId);
      if (org) {
        return { type: 'organization', name: org.name };
      }
    }

    return null;
  }
}
