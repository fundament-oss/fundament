import { Component, inject, signal, OnInit, ChangeDetectionStrategy, ViewChild, ElementRef } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute, Router } from '@angular/router';
import { NgIconComponent, provideIcons } from '@ng-icons/core';
import { tablerX, tablerPencil, tablerCheck, tablerAlertTriangle } from '@ng-icons/tabler-icons';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';
import { TitleService } from '../title.service';
import { ToastService } from '../toast.service';
import { OrganizationDataService } from '../organization-data.service';
import ModalComponent from '../modal/modal.component';
import { PROJECT } from '../../connect/tokens';
import {
  GetProjectRequestSchema,
  UpdateProjectRequestSchema,
  DeleteProjectRequestSchema,
  type Project,
} from '../../generated/v1/project_pb';
import { formatDate as formatDateUtil } from '../utils/date-format';

@Component({
  selector: 'app-project-settings',
  imports: [FormsModule, NgIconComponent, ModalComponent],
  viewProviders: [
    provideIcons({
      tablerX,
      tablerPencil,
      tablerCheck,
      tablerAlertTriangle,
    }),
  ],
  templateUrl: './project-settings.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class ProjectSettingsComponent implements OnInit {
  private titleService = inject(TitleService);

  private route = inject(ActivatedRoute);

  private router = inject(Router);

  private projectClient = inject(PROJECT);

  private toastService = inject(ToastService);

  private organizationDataService = inject(OrganizationDataService);

  projectId = signal<string>('');

  project = signal<Project | undefined>(undefined);

  isEditing = signal(false);

  editingName = signal('');

  loading = signal(false);

  error = signal<string | null>(null);

  @ViewChild('nameInput') nameInput?: ElementRef<HTMLInputElement>;

  showDeleteModal = signal<boolean>(false);

  constructor() {
    this.titleService.setTitle('Project Settings');
  }

  async ngOnInit() {
    const id = this.route.snapshot.params['id'];
    if (id) {
      this.projectId.set(id);
      await this.loadProject(id);
    }
  }

  private async loadProject(projectId: string) {
    this.loading.set(true);
    this.error.set(null);
    try {
      const request = create(GetProjectRequestSchema, { projectId });
      const response = await firstValueFrom(this.projectClient.getProject(request));
      if (response.project) {
        this.project.set(response.project);
      }
    } catch (err) {
      this.error.set(
        err instanceof Error ? `Failed to load project: ${err.message}` : 'Failed to load project',
      );
    } finally {
      this.loading.set(false);
    }
  }

  startEdit() {
    const currentProject = this.project();
    if (currentProject) {
      this.isEditing.set(true);
      this.editingName.set(currentProject.name);

      setTimeout(() => {
        this.nameInput?.nativeElement.focus();
      });
    }
  }

  cancelEdit() {
    this.isEditing.set(false);
    this.editingName.set('');
  }

  async saveEdit() {
    const currentProject = this.project();
    const nameToSave = this.editingName();

    if (!nameToSave.trim() || !currentProject) {
      return;
    }

    this.loading.set(true);
    this.error.set(null);

    try {
      const request = create(UpdateProjectRequestSchema, {
        projectId: currentProject.id,
        name: nameToSave.trim(),
      });
      await firstValueFrom(this.projectClient.updateProject(request));

      // Update the local project with the new name
      this.project.set({
        ...currentProject,
        name: nameToSave.trim(),
      });
      this.organizationDataService.updateProjectName(currentProject.id, nameToSave.trim());
      this.isEditing.set(false);
      this.editingName.set('');
    } catch (err) {
      this.error.set(
        err instanceof Error
          ? `Failed to update project name: ${err.message}`
          : 'Failed to update project name',
      );
    } finally {
      this.loading.set(false);
    }
  }

  async deleteProject() {
    const currentProject = this.project();
    if (!currentProject) return;

    try {
      const request = create(DeleteProjectRequestSchema, {
        projectId: currentProject.id,
      });

      await firstValueFrom(this.projectClient.deleteProject(request));

      this.showDeleteModal.set(false);
      this.toastService.info(`Project '${currentProject.name}' deleted`);

      // Reload organization data to update the selector modal
      await this.organizationDataService.loadOrganizationData();

      this.router.navigate(['/projects']);
    } catch (err) {
      this.showDeleteModal.set(false);
      this.error.set(
        err instanceof Error
          ? `Failed to delete project: ${err.message}`
          : 'Failed to delete project',
      );
    }
  }

  readonly formatDate = formatDateUtil;
}
