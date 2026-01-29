import { Component, inject, ViewChild, ElementRef, signal, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute } from '@angular/router';
import { TitleService } from '../title.service';
import { NgIconComponent, provideIcons } from '@ng-icons/core';
import { tablerX, tablerPencil, tablerCheck } from '@ng-icons/tabler-icons';
import { BreadcrumbComponent, BreadcrumbSegment } from '../breadcrumb/breadcrumb.component';

interface Project {
  id: string;
  name: string;
  created: string;
}

@Component({
  selector: 'app-project-settings',
  standalone: true,
  imports: [CommonModule, FormsModule, NgIconComponent, BreadcrumbComponent],
  viewProviders: [
    provideIcons({
      tablerX,
      tablerPencil,
      tablerCheck,
    }),
  ],
  templateUrl: './project-settings.component.html',
})
export class ProjectSettingsComponent implements OnInit {
  private titleService = inject(TitleService);
  private route = inject(ActivatedRoute);

  @ViewChild('nameInput') nameInput?: ElementRef<HTMLInputElement>;

  projectId = signal<string>('');

  // Mock project data
  project = signal<Project>({
    id: '550e8400-e29b-41d4-a716-446655440000',
    name: 'mobile-app-backend',
    created: new Date().toISOString(),
  });

  isEditing = signal(false);
  editingName = signal('');
  loading = signal(false);
  error = signal<string | null>(null);

  constructor() {
    this.titleService.setTitle('Project Settings');
  }

  ngOnInit() {
    const id = this.route.snapshot.params['id'];
    if (id) {
      this.projectId.set(id);
    }
  }

  get breadcrumbSegments(): BreadcrumbSegment[] {
    const segments: BreadcrumbSegment[] = [];

    const currentProject = this.project();
    if (currentProject?.name) {
      segments.push({
        label: currentProject.name,
        route: `/projects/${this.projectId()}`,
      });
    }

    segments.push({ label: 'Settings' });

    return segments;
  }

  startEdit() {
    const currentProject = this.project();
    if (currentProject) {
      this.isEditing.set(true);
      this.editingName.set(currentProject.name);

      // Focus the input field after Angular updates the view
      setTimeout(() => {
        this.nameInput?.nativeElement.focus();
      });
    }
  }

  cancelEdit() {
    this.isEditing.set(false);
    this.editingName.set('');
  }

  saveEdit() {
    const currentProject = this.project();
    const nameToSave = this.editingName();

    if (!nameToSave.trim() || !currentProject) {
      return;
    }

    this.loading.set(true);
    this.error.set(null);

    // Simulate API call with timeout
    setTimeout(() => {
      // Update the local project with the new name
      this.project.set({
        ...currentProject,
        name: nameToSave.trim(),
      });
      this.isEditing.set(false);
      this.editingName.set('');
      this.loading.set(false);
    }, 500);
  }

  formatDate(dateString: string): string {
    try {
      return new Date(dateString).toLocaleDateString('en-US', {
        year: 'numeric',
        month: 'long',
        day: 'numeric',
      });
    } catch {
      return dateString;
    }
  }
}
