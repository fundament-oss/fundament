import {
  Component,
  Input,
  Output,
  EventEmitter,
  signal,
  HostListener,
  ViewChild,
  ElementRef,
  AfterViewInit,
  OnChanges,
} from '@angular/core';
import { CommonModule } from '@angular/common';
import { NgIconComponent, provideIcons } from '@ng-icons/core';
import {
  tablerX,
  tablerSearch,
  tablerChevronRight,
  tablerFolder,
  tablerBracketsContain,
  tablerBuilding,
} from '@ng-icons/tabler-icons';

interface Namespace {
  id: string;
  name: string;
}

interface Project {
  id: string;
  name: string;
  namespaces: Namespace[];
}

interface Organization {
  id: string;
  name: string;
  projects: Project[];
}

@Component({
  selector: 'app-selector-modal',
  standalone: true,
  imports: [CommonModule, NgIconComponent],
  viewProviders: [
    provideIcons({
      tablerX,
      tablerSearch,
      tablerChevronRight,
      tablerFolder,
      tablerBracketsContain,
      tablerBuilding,
    }),
  ],
  templateUrl: './selector-modal.component.html',
})
export class SelectorModalComponent implements AfterViewInit, OnChanges {
  @Input() show = false;
  @Input() organizations: Organization[] = [];
  @Input() selectedOrgId: string | null = null;
  @Input() selectedProjectId: string | null = null;
  @Input() selectedNamespaceId: string | null = null;
  @Input() expandedOrganizations: Set<string> = new Set<string>();
  @Input() expandedProjects: Set<string> = new Set<string>();

  @Output() closeModal = new EventEmitter<void>();
  @Output() selectOrganization = new EventEmitter<string>();
  @Output() selectProject = new EventEmitter<string>();
  @Output() selectNamespace = new EventEmitter<string>();

  @ViewChild('searchInput') searchInput?: ElementRef<HTMLInputElement>;

  filterText = signal('');
  filterInputValue = signal('');

  ngAfterViewInit(): void {
    if (this.show) {
      this.focusSearchInput();
    }
  }

  ngOnChanges(): void {
    if (this.show) {
      // Use setTimeout to ensure the DOM is ready
      setTimeout(() => this.focusSearchInput(), 0);
    }
  }

  private focusSearchInput(): void {
    this.searchInput?.nativeElement.focus();
  }

  @HostListener('document:keydown', ['$event'])
  handleEscapeKey(event: KeyboardEvent): void {
    if (this.show && event.key === 'Escape') {
      event.preventDefault();
      this.onClose();
    }
  }

  onClose(): void {
    this.closeModal.emit();
  }

  onBackdropClick(event: Event): void {
    // Close modal when clicking on the backdrop
    if (event.target === event.currentTarget) {
      this.onClose();
    }
  }

  onModalContentClick(event: Event): void {
    // Stop propagation to prevent backdrop click from firing
    event.stopPropagation();
  }

  updateFilter(event: Event): void {
    const input = event.target as HTMLInputElement;
    this.filterInputValue.set(input.value);
    this.filterText.set(input.value.toLowerCase());
  }

  onSelectOrganization(orgId: string): void {
    this.selectOrganization.emit(orgId);
  }

  onSelectProject(projectId: string): void {
    this.selectProject.emit(projectId);
  }

  onSelectNamespace(namespaceId: string): void {
    this.selectNamespace.emit(namespaceId);
  }

  isOrganizationExpanded(orgId: string): boolean {
    return this.expandedOrganizations.has(orgId);
  }

  isProjectExpanded(projectId: string): boolean {
    return this.expandedProjects.has(projectId);
  }

  isOrganizationSelected(orgId: string): boolean {
    return this.selectedOrgId === orgId;
  }

  isProjectSelected(projectId: string): boolean {
    return this.selectedProjectId === projectId;
  }

  isNamespaceSelected(namespaceId: string): boolean {
    return this.selectedNamespaceId === namespaceId;
  }

  filteredOrganizations(): Organization[] {
    const filterText = this.filterText();
    if (!filterText) {
      return this.organizations;
    }

    return this.organizations
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
          .filter((p): p is Project => p !== null);

        if (orgMatches || filteredProjects.length > 0) {
          return {
            ...org,
            projects: orgMatches ? org.projects : filteredProjects,
          };
        }
        return null;
      })
      .filter((org): org is Organization => org !== null);
  }
}
