import { Component, inject, signal, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterLink, ActivatedRoute, Router } from '@angular/router';
import { TitleService } from '../title.service';
import { ToastService } from '../toast.service';
import { PROJECT } from '../../connect/tokens';
import { create } from '@bufbuild/protobuf';
import {
  GetProjectRequestSchema,
  ListProjectNamespacesRequestSchema,
  DeleteProjectRequestSchema,
  Project,
  ProjectNamespace,
} from '../../generated/v1/project_pb';
import { firstValueFrom } from 'rxjs';
import { ErrorIconComponent, WarningIconComponent } from '../icons';

@Component({
  selector: 'app-project-detail',
  standalone: true,
  imports: [CommonModule, RouterLink, ErrorIconComponent, WarningIconComponent],
  templateUrl: './project-detail.component.html',
})
export class ProjectDetailComponent implements OnInit {
  private titleService = inject(TitleService);
  private route = inject(ActivatedRoute);
  private router = inject(Router);
  private client = inject(PROJECT);
  private toastService = inject(ToastService);

  errorMessage = signal<string | null>(null);
  isLoading = signal<boolean>(true);
  showDeleteModal = signal<boolean>(false);

  project = signal<Project | null>(null);
  namespaces = signal<ProjectNamespace[]>([]);

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

      // Fetch attached namespaces
      const namespacesRequest = create(ListProjectNamespacesRequestSchema, { projectId });
      const namespacesResponse = await firstValueFrom(this.client.listNamespaces(namespacesRequest));
      this.namespaces.set(namespacesResponse.namespaces);
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
}
