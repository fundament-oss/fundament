import { Component, signal, HostListener, inject, OnInit } from '@angular/core';
import { RouterOutlet, RouterLink, RouterLinkActive, Router } from '@angular/router';
import { CommonModule } from '@angular/common';
import { AuthnApiService } from './authn-api.service';
import type { User } from '../generated/authn/v1/authn_pb';
import { ToastService } from './toast.service';
import { versionMismatch$ } from './app.config';
import { SelectorModalComponent } from './selector-modal/selector-modal.component';
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
  expandedOrganizations = signal<Set<string>>(new Set(['org-1', 'org-2', 'org-3']));
  expandedProjects = signal<Set<string>>(
    new Set(['proj-1', 'proj-2', 'proj-3', 'proj-4', 'proj-5']),
  );
  selectedOrgId = signal<string | null>(null);
  selectedProjectId = signal<string | null>(null);
  selectedNamespaceId = signal<string | null>(null);

  // Mock data for nested selector
  mockOrganizations = signal([
    {
      id: 'org-1',
      name: 'Acme Corporation',
      projects: [
        {
          id: 'proj-1',
          name: 'e-commerce-platform',
          namespaces: [
            { id: 'ns-1', name: 'production' },
            { id: 'ns-2', name: 'staging' },
            { id: 'ns-3', name: 'development' },
          ],
        },
        {
          id: 'proj-2',
          name: 'analytics-service',
          namespaces: [
            { id: 'ns-4', name: 'production' },
            { id: 'ns-5', name: 'staging' },
          ],
        },
      ],
    },
    {
      id: 'org-2',
      name: 'TechStart Inc',
      projects: [
        {
          id: 'proj-3',
          name: 'mobile-app-backend',
          namespaces: [
            { id: 'ns-6', name: 'production' },
            { id: 'ns-7', name: 'qa' },
            { id: 'ns-8', name: 'development' },
          ],
        },
        {
          id: 'proj-4',
          name: 'payment-gateway',
          namespaces: [{ id: 'ns-9', name: 'production' }],
        },
      ],
    },
    {
      id: 'org-3',
      name: 'Global Dynamics',
      projects: [
        {
          id: 'proj-5',
          name: 'internal-tools',
          namespaces: [
            { id: 'ns-10', name: 'production' },
            { id: 'ns-11', name: 'testing' },
          ],
        },
      ],
    },
  ]);

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

    // Initialize selector with first organization selected
    const firstOrg = this.mockOrganizations()[0];
    if (firstOrg) {
      this.selectedOrgId.set(firstOrg.id);
    }
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
    // If already selected, toggle collapse/expand
    if (this.selectedOrgId() === orgId) {
      this.toggleOrganizationExpansion(orgId);
    } else {
      // Select and ensure it's expanded
      this.selectedOrgId.set(orgId);
      this.selectedProjectId.set(null);
      this.selectedNamespaceId.set(null);

      // Expand if not already expanded
      const expanded = this.expandedOrganizations();
      if (!expanded.has(orgId)) {
        const newExpanded = new Set(expanded);
        newExpanded.add(orgId);
        this.expandedOrganizations.set(newExpanded);
      }

      // Close modal
      this.selectorModalOpen.set(false);
    }
  }

  selectProjectItem(projectId: string) {
    // If already selected, toggle collapse/expand
    if (this.selectedProjectId() === projectId) {
      this.toggleProjectExpansion(projectId);
    } else {
      // Select and ensure it's expanded
      this.selectedProjectId.set(projectId);
      this.selectedOrgId.set(null);
      this.selectedNamespaceId.set(null);

      // Expand if not already expanded
      const expanded = this.expandedProjects();
      if (!expanded.has(projectId)) {
        const newExpanded = new Set(expanded);
        newExpanded.add(projectId);
        this.expandedProjects.set(newExpanded);
      }

      // Close modal
      this.selectorModalOpen.set(false);
    }
  }

  selectNamespaceItem(namespaceId: string) {
    this.selectedNamespaceId.set(namespaceId);
    this.selectedOrgId.set(null);

    // Find the project that contains this namespace
    let projectId: string | null = null;
    for (const org of this.mockOrganizations()) {
      for (const project of org.projects) {
        if (project.namespaces.some((ns) => ns.id === namespaceId)) {
          projectId = project.id;
          break;
        }
      }
      if (projectId) break;
    }

    this.selectedProjectId.set(projectId);

    // Close modal
    this.selectorModalOpen.set(false);
  }

  private toggleOrganizationExpansion(orgId: string) {
    const expanded = this.expandedOrganizations();
    const newExpanded = new Set(expanded);
    if (newExpanded.has(orgId)) {
      newExpanded.delete(orgId);
    } else {
      newExpanded.add(orgId);
    }
    this.expandedOrganizations.set(newExpanded);
  }

  private toggleProjectExpansion(projectId: string) {
    const expanded = this.expandedProjects();
    const newExpanded = new Set(expanded);
    if (newExpanded.has(projectId)) {
      newExpanded.delete(projectId);
    } else {
      newExpanded.add(projectId);
    }
    this.expandedProjects.set(newExpanded);
  }

  isOrganizationExpanded(orgId: string): boolean {
    return this.expandedOrganizations().has(orgId);
  }

  isProjectExpanded(projectId: string): boolean {
    return this.expandedProjects().has(projectId);
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
      for (const org of this.mockOrganizations()) {
        for (const project of org.projects) {
          const namespace = project.namespaces.find((ns) => ns.id === namespaceId);
          if (namespace) {
            return { type: 'namespace', name: namespace.name };
          }
        }
      }
    } else if (selectedType === 'project') {
      const projectId = this.selectedProjectId();
      for (const org of this.mockOrganizations()) {
        const project = org.projects.find((p) => p.id === projectId);
        if (project) {
          return { type: 'project', name: project.name };
        }
      }
    } else if (selectedType === 'organization') {
      const orgId = this.selectedOrgId();
      const org = this.mockOrganizations().find((o) => o.id === orgId);
      if (org) {
        return { type: 'organization', name: org.name };
      }
    }

    return null;
  }
}
