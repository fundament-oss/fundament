import { Component, inject, signal, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterLink, RouterLinkActive, ActivatedRoute } from '@angular/router';
import { TitleService } from '../title.service';
import { NgIcon, provideIcons } from '@ng-icons/core';
import { tablerPlus, tablerPencil, tablerTrash } from '@ng-icons/tabler-icons';
import { BreadcrumbComponent, BreadcrumbSegment } from '../breadcrumb/breadcrumb.component';
import { OrganizationDataService } from '../organization-data.service';

@Component({
  selector: 'app-project-members',
  standalone: true,
  imports: [CommonModule, RouterLink, RouterLinkActive, NgIcon, BreadcrumbComponent],
  viewProviders: [
    provideIcons({
      tablerPlus,
      tablerPencil,
      tablerTrash,
    }),
  ],
  templateUrl: './project-members.component.html',
})
export class ProjectMembersComponent implements OnInit {
  private titleService = inject(TitleService);
  private route = inject(ActivatedRoute);
  private organizationDataService = inject(OrganizationDataService);

  projectId = signal<string>('');
  projectName = signal<string>('');

  // Members data for the project
  members = {
    projects: [
      {
        name: 'Project #1',
        users: [
          { name: 'Alice Johnson', role: 'Project admin' },
          { name: 'Bob Johnson', role: 'Project member' },
          { name: 'Charlie Brown', role: 'Project member' },
        ],
      },
      {
        name: 'Project #2',
        users: [
          { name: 'David Wilson', role: 'Project admin' },
          { name: 'Emma Davis', role: 'Project member' },
        ],
      },
      {
        name: 'Project #3',
        users: [
          { name: 'Frank Miller', role: 'Project member' },
          { name: 'Grace Lee', role: 'Project member' },
        ],
      },
    ],
  };

  constructor() {
    this.titleService.setTitle('Project members');
  }

  ngOnInit() {
    const id = this.route.snapshot.params['id'];
    if (id) {
      this.projectId.set(id);
      // Find the actual project name from organization data
      const orgs = this.organizationDataService.organizations();
      for (const org of orgs) {
        const project = org.projects.find((p) => p.id === id);
        if (project) {
          this.projectName.set(project.name);
          break;
        }
      }
    }
  }

  get breadcrumbSegments(): BreadcrumbSegment[] {
    const segments: BreadcrumbSegment[] = [];

    if (this.projectName()) {
      segments.push({
        label: this.projectName(),
        route: `/projects/${this.projectId()}`,
      });
    }

    segments.push({ label: 'Members' });

    return segments;
  }
}
