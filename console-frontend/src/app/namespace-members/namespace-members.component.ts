import { Component, inject, signal, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { ActivatedRoute } from '@angular/router';
import { TitleService } from '../title.service';
import { BreadcrumbComponent, BreadcrumbSegment } from '../breadcrumb/breadcrumb.component';

@Component({
  selector: 'app-namespace-members',
  standalone: true,
  imports: [CommonModule, BreadcrumbComponent],
  templateUrl: './namespace-members.component.html',
})
export class NamespaceMembersComponent implements OnInit {
  private titleService = inject(TitleService);
  private route = inject(ActivatedRoute);

  projectId = signal<string>('');
  namespaceId = signal<string>('');
  projectName = signal<string>(''); // Mock project name
  namespaceName = signal<string>(''); // Mock namespace name

  constructor() {
    this.titleService.setTitle('Namespace Members');
  }

  ngOnInit() {
    const id = this.route.snapshot.params['id'];
    const nsId = this.route.snapshot.params['namespaceId'];
    if (id) {
      this.projectId.set(id);
      // Mock project name - in real app, this would be fetched from API
      this.projectName.set('Project Alpha');
    }
    if (nsId) {
      this.namespaceId.set(nsId);
      // Mock namespace name - in real app, this would be fetched from API
      this.namespaceName.set('production');
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
