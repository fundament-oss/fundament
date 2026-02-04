import { Component, inject, signal, OnInit, ChangeDetectionStrategy } from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterLink } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { timestampDate, type Timestamp } from '@bufbuild/protobuf/wkt';
import { NgIcon, provideIcons } from '@ng-icons/core';
import { tablerPlus, tablerChevronRight } from '@ng-icons/tabler-icons';
import { tablerCircleXFill } from '@ng-icons/tabler-icons/fill';
import { firstValueFrom } from 'rxjs';
import { BreadcrumbComponent } from '../breadcrumb/breadcrumb.component';
import { LoadingIndicatorComponent } from '../icons';
import { TitleService } from '../title.service';
import { PROJECT } from '../../connect/tokens';
import { ListProjectsRequestSchema, Project } from '../../generated/v1/project_pb';
import { formatDate as formatDateUtil } from '../utils/date-format';

@Component({
  selector: 'app-projects',
  imports: [CommonModule, RouterLink, NgIcon, LoadingIndicatorComponent, BreadcrumbComponent],
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

  readonly formatDate = formatDateUtil;

  timestampToDate(timestamp: Timestamp | undefined): string | undefined {
    if (!timestamp) return undefined;
    return timestampDate(timestamp).toISOString();
  }
}
