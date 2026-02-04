import {
  Component,
  Input,
  Output,
  EventEmitter,
  signal,
  ViewChild,
  ElementRef,
  AfterViewInit,
  OnChanges,
  ChangeDetectionStrategy,
} from '@angular/core';
import { CommonModule } from '@angular/common';
import { NgIconComponent, provideIcons } from '@ng-icons/core';
import {
  tablerSearch,
  tablerFolder,
  tablerBracketsContain,
  tablerBuilding,
} from '@ng-icons/tabler-icons';
import { ModalComponent } from '../modal/modal.component';

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
  imports: [CommonModule, NgIconComponent, ModalComponent],
  viewProviders: [
    provideIcons({
      tablerSearch,
      tablerFolder,
      tablerBracketsContain,
      tablerBuilding,
    }),
  ],
  templateUrl: './selector-modal.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class SelectorModalComponent implements AfterViewInit, OnChanges {
  @Input() show = false;
  @Input() organizations: Organization[] = [];
  @Input() selectedOrgId: string | null = null;
  @Input() selectedProjectId: string | null = null;
  @Input() selectedNamespaceId: string | null = null;

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

  onClose(): void {
    this.closeModal.emit();
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

  isOrganizationSelected(orgId: string): boolean {
    // Only highlight organization if no project or namespace is selected
    return this.selectedOrgId === orgId && !this.selectedProjectId && !this.selectedNamespaceId;
  }

  isProjectSelected(projectId: string): boolean {
    // Only highlight project if it's selected and no namespace is selected
    return this.selectedProjectId === projectId && !this.selectedNamespaceId;
  }

  isNamespaceSelected(namespaceId: string): boolean {
    // Highlight namespace if it's selected (takes precedence)
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
