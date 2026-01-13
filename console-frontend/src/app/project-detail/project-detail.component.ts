import { Component, inject, signal, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterLink, ActivatedRoute, Router } from '@angular/router';
import { ReactiveFormsModule, FormBuilder, FormGroup, Validators } from '@angular/forms';
import { TitleService } from '../title.service';
import { ToastService } from '../toast.service';
import { PROJECT } from '../../connect/tokens';
import { create } from '@bufbuild/protobuf';
import {
  GetProjectRequestSchema,
  ListProjectNamespacesRequestSchema,
  DeleteProjectRequestSchema,
  AttachNamespaceRequestSchema,
  DetachNamespaceRequestSchema,
  Project,
  ProjectNamespace,
} from '../../generated/v1/project_pb';
import { firstValueFrom } from 'rxjs';
import {
  ErrorIconComponent,
  WarningIconComponent,
  PlusIconComponent,
  TrashIconComponent,
} from '../icons';

@Component({
  selector: 'app-project-detail',
  standalone: true,
  imports: [
    CommonModule,
    RouterLink,
    ReactiveFormsModule,
    ErrorIconComponent,
    WarningIconComponent,
    PlusIconComponent,
    TrashIconComponent,
  ],
  templateUrl: './project-detail.component.html',
})
export class ProjectDetailComponent implements OnInit {
  private titleService = inject(TitleService);
  private route = inject(ActivatedRoute);
  private router = inject(Router);
  private client = inject(PROJECT);
  private toastService = inject(ToastService);
  private fb = inject(FormBuilder);

  errorMessage = signal<string | null>(null);
  isLoading = signal<boolean>(true);
  showDeleteModal = signal<boolean>(false);
  showAttachModal = signal<boolean>(false);
  isAttaching = signal<boolean>(false);

  project = signal<Project | null>(null);
  namespaces = signal<ProjectNamespace[]>([]);

  attachForm: FormGroup;

  constructor() {
    this.attachForm = this.fb.group({
      namespaceId: ['', [Validators.required]],
    });
  }

  async ngOnInit() {
    const projectId = this.route.snapshot.params['id'];

    try {
      this.isLoading.set(true);
      this.errorMessage.set(null);

      const request = create(GetProjectRequestSchema, { projectId });
      const response = await firstValueFrom(this.client.getProject(request));

      if (!response.project) {
        throw new Error('Project not found');
      }

      this.project.set(response.project);
      this.titleService.setTitle(response.project.name);

      await this.loadNamespaces();
    } catch (error) {
      console.error('Failed to fetch project data:', error);
      this.errorMessage.set(
        error instanceof Error
          ? `Failed to load project: ${error.message}`
          : 'Failed to load project data',
      );
    } finally {
      this.isLoading.set(false);
    }
  }

  private async loadNamespaces(): Promise<void> {
    const currentProject = this.project();
    if (!currentProject) return;

    const namespacesRequest = create(ListProjectNamespacesRequestSchema, {
      projectId: currentProject.id,
    });
    const namespacesResponse = await firstValueFrom(this.client.listNamespaces(namespacesRequest));
    this.namespaces.set(namespacesResponse.namespaces);
  }

  formatDate(dateString?: string): string {
    if (!dateString) {
      return 'Unknown';
    }
    return new Date(dateString).toLocaleDateString('en-US', {
      year: 'numeric',
      month: 'long',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    });
  }

  async attachNamespace(): Promise<void> {
    if (this.attachForm.invalid) {
      this.attachForm.markAllAsTouched();
      return;
    }

    const currentProject = this.project();
    if (!currentProject) return;

    this.isAttaching.set(true);

    try {
      const request = create(AttachNamespaceRequestSchema, {
        projectId: currentProject.id,
        namespaceId: this.attachForm.value.namespaceId.trim(),
      });

      await firstValueFrom(this.client.attachNamespace(request));

      this.showAttachModal.set(false);
      this.attachForm.reset();
      this.toastService.info('Namespace attached successfully');
      await this.loadNamespaces();
    } catch (error) {
      console.error('Failed to attach namespace:', error);
      this.errorMessage.set(
        error instanceof Error
          ? `Failed to attach namespace: ${error.message}`
          : 'Failed to attach namespace',
      );
      this.showAttachModal.set(false);
    } finally {
      this.isAttaching.set(false);
    }
  }

  async detachNamespace(namespaceId: string): Promise<void> {
    const currentProject = this.project();
    if (!currentProject) return;

    try {
      const request = create(DetachNamespaceRequestSchema, {
        projectId: currentProject.id,
        namespaceId,
      });

      await firstValueFrom(this.client.detachNamespace(request));

      this.toastService.info('Namespace detached successfully');
      await this.loadNamespaces();
    } catch (error) {
      console.error('Failed to detach namespace:', error);
      this.errorMessage.set(
        error instanceof Error
          ? `Failed to detach namespace: ${error.message}`
          : 'Failed to detach namespace',
      );
    }
  }

  async deleteProject(): Promise<void> {
    const currentProject = this.project();
    if (!currentProject) return;

    try {
      const request = create(DeleteProjectRequestSchema, {
        projectId: currentProject.id,
      });

      await firstValueFrom(this.client.deleteProject(request));

      this.showDeleteModal.set(false);
      this.toastService.info(`The project '${currentProject.name}' has been deleted`);
      this.router.navigate(['/projects']);
    } catch (error) {
      console.error('Failed to delete project:', error);
      this.showDeleteModal.set(false);
      this.errorMessage.set(
        error instanceof Error
          ? `Failed to delete project: ${error.message}`
          : 'Failed to delete project',
      );
    }
  }

  openAttachModal(): void {
    this.attachForm.reset();
    this.showAttachModal.set(true);
  }
}
