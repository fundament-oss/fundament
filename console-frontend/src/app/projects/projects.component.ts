import { Component, inject, signal, OnInit, ChangeDetectionStrategy } from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterLink } from '@angular/router';
import { TitleService } from '../title.service';
import { PROJECT } from '../../connect/tokens';
import { create } from '@bufbuild/protobuf';
import { type Timestamp, timestampDate } from '@bufbuild/protobuf/wkt';
import { ListProjectsRequestSchema, Project } from '../../generated/v1/project_pb';
import { firstValueFrom } from 'rxjs';
import { NgIcon, provideIcons } from '@ng-icons/core';
import { tablerPlus, tablerChevronRight } from '@ng-icons/tabler-icons';
import { tablerCircleXFill } from '@ng-icons/tabler-icons/fill';
import { LoadingIndicatorComponent } from '../icons';

@Component({
  selector: 'app-projects',
  imports: [CommonModule, RouterLink, NgIcon, LoadingIndicatorComponent],
  viewProviders: [
    provideIcons({
      tablerCircleXFill,
      tablerPlus,
      tablerChevronRight,
    }),
  ],
  templateUrl: './projects.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class ProjectsComponent implements OnInit {
  private titleService = inject(TitleService);
  private client = inject(PROJECT);

  projects = signal<Project[]>([]);
  isLoading = signal<boolean>(true);
  errorMessage = signal<string | null>(null);

  constructor() {
    this.titleService.setTitle('Projects');
  }

  async ngOnInit() {
    await this.loadProjects();
  }

  async loadProjects() {
    try {
      this.isLoading.set(true);
      this.errorMessage.set(null);

      const request = create(ListProjectsRequestSchema, {});
      const response = await firstValueFrom(this.client.listProjects(request));

      this.projects.set(response.projects);
    } catch (error) {
      console.error('Failed to fetch projects:', error);
      this.errorMessage.set(
        error instanceof Error
          ? `Failed to load projects: ${error.message}`
          : 'Failed to load projects',
      );
    } finally {
      this.isLoading.set(false);
    }
  }

  formatDate(timestamp: Timestamp | undefined): string {
    if (!timestamp) return 'Unknown';
    return timestampDate(timestamp).toLocaleDateString('en-US', {
      year: 'numeric',
      month: 'long',
      day: 'numeric',
    });
  }
}
