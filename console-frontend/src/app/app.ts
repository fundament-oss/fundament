import { Component, signal, HostListener, inject, OnInit } from '@angular/core';
import { RouterOutlet, RouterLink, RouterLinkActive, Router } from '@angular/router';
import { CommonModule } from '@angular/common';
import { AuthnApiService } from './authn-api.service';
import type { User } from '../generated/authn/v1/authn_pb';
import { ToastService } from './toast.service';
import { versionMismatch$ } from './app.config';
import {
  WarningIconComponent,
  MenuIconComponent,
  CloseIconComponent,
  MoonIconComponent,
  SunIconComponent,
  ChevronDownIconComponent,
  ChevronRightIconComponent,
  UserCircleIconComponent,
  FundamentLogoIconComponent,
  DashboardIconComponent,
  KubernetesIconComponent,
  FolderIconComponent,
  PuzzleIconComponent,
  UsersIconComponent,
  ChartIconComponent,
  CheckCircleIconComponent,
  ErrorIconComponent,
  InfoCircleIconComponent,
} from './icons';

@Component({
  selector: 'app-root',
  imports: [
    RouterOutlet,
    RouterLink,
    RouterLinkActive,
    CommonModule,
    WarningIconComponent,
    MenuIconComponent,
    CloseIconComponent,
    MoonIconComponent,
    SunIconComponent,
    ChevronDownIconComponent,
    ChevronRightIconComponent,
    UserCircleIconComponent,
    FundamentLogoIconComponent,
    DashboardIconComponent,
    KubernetesIconComponent,
    FolderIconComponent,
    PuzzleIconComponent,
    UsersIconComponent,
    ChartIconComponent,
    CheckCircleIconComponent,
    ErrorIconComponent,
    InfoCircleIconComponent,
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

  // Nested selector state
  selectorFilterText = signal('');
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
    }
  }

  selectNamespaceItem(namespaceId: string) {
    this.selectedNamespaceId.set(namespaceId);
    this.selectedOrgId.set(null);
    this.selectedProjectId.set(null);
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

  updateSelectorFilter(event: Event) {
    const input = event.target as HTMLInputElement;
    this.selectorFilterText.set(input.value.toLowerCase());
  }

  filteredOrganizations() {
    const filterText = this.selectorFilterText();
    if (!filterText) {
      return this.mockOrganizations();
    }

    return this.mockOrganizations()
      .map((org) => {
        const orgMatches = org.name.toLowerCase().includes(filterText);
        const filteredProjects = org.projects
          .map((project) => {
            const projectMatches = project.name.toLowerCase().includes(filterText);
            const filteredNamespaces = project.namespaces.filter((ns) =>
              ns.name.toLowerCase().includes(filterText),
            );

            if (projectMatches || filteredNamespaces.length > 0) {
              return {
                ...project,
                namespaces: projectMatches ? project.namespaces : filteredNamespaces,
              };
            }
            return null;
          })
          .filter((p) => p !== null);

        if (orgMatches || filteredProjects.length > 0) {
          return {
            ...org,
            projects: orgMatches ? org.projects : filteredProjects,
          };
        }
        return null;
      })
      .filter((org) => org !== null);
  }

  getSelectedType(): 'organization' | 'project' | 'namespace' | null {
    if (this.selectedNamespaceId()) return 'namespace';
    if (this.selectedProjectId()) return 'project';
    if (this.selectedOrgId()) return 'organization';
    return null;
  }

  getSettingsHeader(): string {
    const type = this.getSelectedType();
    if (type === 'organization') return 'Organization settings';
    if (type === 'project') return 'Project settings';
    if (type === 'namespace') return 'Namespace settings';
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
}
