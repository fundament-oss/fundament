import {
  Component,
  Input,
  Output,
  EventEmitter,
  signal,
  ChangeDetectionStrategy,
} from '@angular/core';
import { CommonModule } from '@angular/common';
import { NgIconComponent, provideIcons } from '@ng-icons/core';
import { tablerSearch, tablerFolder, tablerBuilding } from '@ng-icons/tabler-icons';
import { ModalComponent } from '../modal/modal.component';

interface Project {
  id: string;
  name: string;
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
      tablerBuilding,
    }),
  ],
  templateUrl: './selector-modal.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class SelectorModalComponent {
  @Input() show = false;
  @Input() organizations: Organization[] = [];
  @Input() selectedOrgId: string | null = null;
  @Input() selectedProjectId: string | null = null;

  @Output() closeModal = new EventEmitter<void>();
  @Output() selectOrganization = new EventEmitter<string>();
  @Output() selectProject = new EventEmitter<string>();

  filterText = signal('');
  filterInputValue = signal('');

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

  isOrganizationSelected(orgId: string): boolean {
    // Only highlight organization if no project is selected
    return this.selectedOrgId === orgId && !this.selectedProjectId;
  }

  isProjectSelected(projectId: string): boolean {
    return this.selectedProjectId === projectId;
  }

  filteredOrganizations(): Organization[] {
    const filterText = this.filterText();
    if (!filterText) {
      return this.organizations;
    }

    return this.organizations
      .map((org) => {
        const orgMatches = org.name.toLowerCase().includes(filterText);
        const filteredProjects = org.projects.filter((project) =>
          project.name.toLowerCase().includes(filterText),
        );

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
