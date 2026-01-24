import { Component, inject, ViewChild, ElementRef, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { TitleService } from '../title.service';
import { CheckmarkIconComponent, CloseIconComponent, EditIconComponent } from '../icons';

interface Project {
  id: string;
  name: string;
  created: string;
}

@Component({
  selector: 'app-project-settings',
  standalone: true,
  imports: [
    CommonModule,
    FormsModule,
    CheckmarkIconComponent,
    CloseIconComponent,
    EditIconComponent,
  ],
  templateUrl: './project-settings.component.html',
})
export class ProjectSettingsComponent {
  private titleService = inject(TitleService);

  @ViewChild('nameInput') nameInput?: ElementRef<HTMLInputElement>;

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
