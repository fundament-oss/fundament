import { Component, inject, OnInit, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { ActivatedRoute } from '@angular/router';
import { TitleService } from '../title.service';
import { BreadcrumbComponent, BreadcrumbSegment } from '../breadcrumb/breadcrumb.component';

@Component({
  selector: 'app-namespaces',
  standalone: true,
  imports: [CommonModule, BreadcrumbComponent],
  templateUrl: './namespaces.component.html',
})
export class NamespacesComponent implements OnInit {
  private titleService = inject(TitleService);
  private route = inject(ActivatedRoute);

  projectId = signal<string>('');
  projectName = signal<string>(''); // Mock project name

  constructor() {
    this.titleService.setTitle('Namespaces');
  }

  ngOnInit() {
    const id = this.route.snapshot.params['id'];
    if (id) {
      this.projectId.set(id);
      // Mock project name - in real app, this would be fetched from API
      this.projectName.set('Project Alpha');
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

    segments.push({ label: 'Namespaces' });

    return segments;
  }
}
