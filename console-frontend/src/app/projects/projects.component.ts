import { Component, inject, signal, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterLink } from '@angular/router';
import { TitleService } from '../title.service';
import { PlusIconComponent, EyeIconComponent, ErrorIconComponent } from '../icons';
import { PROJECT } from '../../connect/tokens';
import { Project } from '../../generated/v1/project_pb';
import { firstValueFrom } from 'rxjs';

@Component({
  selector: 'app-projects',
  standalone: true,
  imports: [CommonModule, RouterLink, PlusIconComponent, EyeIconComponent, ErrorIconComponent],
  templateUrl: './projects.component.html',
})
export class ProjectsComponent implements OnInit {
  private titleService = inject(TitleService);
  private client = inject(PROJECT);

  projects = signal<Project[]>([]);
  errorMessage = signal<string>('');
  namespaceCounts = signal<Map<string, number>>(new Map());

  constructor() {
    this.titleService.setTitle('Projects');
  }

  async ngOnInit() {
    try {
      const response = await firstValueFrom(this.client.listProjects({}));
      this.projects.set(response.projects);

      // Fetch namespace counts for each project
      for (const project of response.projects) {
        try {
          const namespacesResponse = await firstValueFrom(
            this.client.listNamespaces({ projectId: project.id }),
          );
          const counts = new Map(this.namespaceCounts());
          counts.set(project.id, namespacesResponse.namespaces.length);
          this.namespaceCounts.set(counts);
        } catch (error) {
          console.error(`Failed to load namespaces for project ${project.id}:`, error);
        }
      }
    } catch (error) {
      console.error('Failed to load projects:', error);
      this.errorMessage.set(
        error instanceof Error ? error.message : 'Failed to load projects. Please try again later.',
      );
    }
  }

  getNamespaceCount(projectId: string): number {
    return this.namespaceCounts().get(projectId) || 0;
  }

  formatDate(dateString?: string): string {
    if (!dateString) {
      return 'Unknown';
    }
    try {
      const date = new Date(dateString);
      return date.toLocaleDateString('en-US', {
        year: 'numeric',
        month: 'short',
        day: 'numeric',
      });
    } catch {
      return 'Invalid date';
    }
  }
}
