import { Component, inject, signal, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { ActivatedRoute } from '@angular/router';
import { TitleService } from '../title.service';
import { BreadcrumbComponent, BreadcrumbSegment } from '../breadcrumb/breadcrumb.component';
import { OrganizationDataService } from '../organization-data.service';

@Component({
  selector: 'app-namespace-members',
  standalone: true,
  imports: [CommonModule, BreadcrumbComponent],
  templateUrl: './namespace-members.component.html',
})
export class NamespaceMembersComponent implements OnInit {
  private titleService = inject(TitleService);
  private route = inject(ActivatedRoute);
  private organizationDataService = inject(OrganizationDataService);

  projectId = signal<string>('');
  namespaceId = signal<string>('');
  projectName = signal<string>('');
  namespaceName = signal<string>('');

  constructor() {
    this.titleService.setTitle('Namespace Members');
  }

  ngOnInit() {
    const id = this.route.snapshot.params['id'];
    const nsId = this.route.snapshot.params['namespaceId'];
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
    if (nsId) {
      this.namespaceId.set(nsId);
      // Find the actual namespace name from organization data
      const orgs = this.organizationDataService.organizations();
      for (const org of orgs) {
        for (const project of org.projects) {
          const namespace = project.namespaces.find((ns) => ns.id === nsId);
          if (namespace) {
            this.namespaceName.set(namespace.name);
            break;
          }
        }
        if (this.namespaceName()) break;
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

    if (this.namespaceName()) {
      segments.push({
        label: this.namespaceName(),
        route: `/projects/${this.projectId()}/namespaces/${this.namespaceId()}`,
      });
    }

    segments.push({ label: 'Members' });

    return segments;
  }
}
