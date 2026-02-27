import {
  Component,
  input,
  output,
  signal,
  computed,
  effect,
  ChangeDetectionStrategy,
} from '@angular/core';
import { NgIconComponent, provideIcons } from '@ng-icons/core';
import { tablerSearch, tablerFolder, tablerBuilding } from '@ng-icons/tabler-icons';
import ModalComponent from '../modal/modal.component';

interface Project {
  id: string;
  name: string;
}

interface Organization {
  id: string;
  displayName: string;
  projects: Project[];
}

@Component({
  selector: 'app-selector-modal',
  imports: [NgIconComponent, ModalComponent],
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
export default class SelectorModalComponent {
  show = input(false);

  organizations = input<Organization[]>([]);

  selectedOrgId = input<string | null>(null);

  selectedProjectId = input<string | null>(null);

  closeModal = output();

  selectOrganization = output<string>();

  selectProject = output<string>();

  filterText = signal('');

  filterInputValue = signal('');

  // Reset filter when modal opens
  private resetOnOpen = effect(() => {
    if (this.show()) {
      this.filterText.set('');
      this.filterInputValue.set('');
    }
  });

  filteredOrganizations = computed(() => {
    const filterText = this.filterText();
    const orgs = this.organizations();
    if (!filterText) {
      return orgs;
    }

    return orgs
      .map((org) => {
        const orgMatches = org.displayName.toLowerCase().includes(filterText);
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
  });

  onClose(): void {
    this.closeModal.emit();
  }

  updateFilter(event: Event): void {
    const inputEl = event.target as HTMLInputElement;
    this.filterInputValue.set(inputEl.value);
    this.filterText.set(inputEl.value.toLowerCase());
  }

  onSelectOrganization(orgId: string): void {
    this.selectOrganization.emit(orgId);
  }

  onSelectProject(projectId: string): void {
    this.selectProject.emit(projectId);
  }

  isOrganizationSelected(orgId: string): boolean {
    return this.selectedOrgId() === orgId && !this.selectedProjectId();
  }

  isProjectSelected(projectId: string): boolean {
    return this.selectedProjectId() === projectId;
  }
}
