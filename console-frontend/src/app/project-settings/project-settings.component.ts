import { Component, inject, ViewChild, ElementRef, signal, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute } from '@angular/router';
import { TitleService } from '../title.service';
import { NgIconComponent, provideIcons } from '@ng-icons/core';
import { tablerX, tablerPencil, tablerCheck } from '@ng-icons/tabler-icons';
import { BreadcrumbComponent, BreadcrumbSegment } from '../breadcrumb/breadcrumb.component';
import { PROJECT } from '../../connect/tokens';
import { create } from '@bufbuild/protobuf';
import {
  GetProjectRequestSchema,
  UpdateProjectRequestSchema,
  type Project,
} from '../../generated/v1/project_pb';
import { firstValueFrom } from 'rxjs';
import { timestampDate, type Timestamp } from '@bufbuild/protobuf/wkt';

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
  private projectClient = inject(PROJECT);

  @ViewChild('nameInput') nameInput?: ElementRef<HTMLInputElement>;

  projectId = signal<string>('');
  project = signal<Project | undefined>(undefined);

  isEditing = signal(false);
  editingName = signal('');
  loading = signal(false);
  error = signal<string | null>(null);

  constructor() {
    this.titleService.setTitle('Project Settings');
  }

  async ngOnInit() {
    const id = this.route.snapshot.params['id'];
    if (id) {
      this.projectId.set(id);
      await this.loadProject(id);
    }
  }

  private async loadProject(projectId: string) {
    this.loading.set(true);
    this.error.set(null);
    try {
      const request = create(GetProjectRequestSchema, { projectId });
      const response = await firstValueFrom(this.projectClient.getProject(request));
      if (response.project) {
        this.project.set(response.project);
      }
    } catch (err) {
      console.error('Error loading project:', err);
      this.error.set('Failed to load project');
    } finally {
      this.loading.set(false);
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

  async saveEdit() {
    const currentProject = this.project();
    const nameToSave = this.editingName();

    if (!nameToSave.trim() || !currentProject) {
      return;
    }

    this.loading.set(true);
    this.error.set(null);

    try {
      const request = create(UpdateProjectRequestSchema, {
        projectId: currentProject.id,
        name: nameToSave.trim(),
      });
      await firstValueFrom(this.projectClient.updateProject(request));

      // Update the local project with the new name
      this.project.set({
        ...currentProject,
        name: nameToSave.trim(),
      });
      this.isEditing.set(false);
      this.editingName.set('');
    } catch (err) {
      console.error('Error updating project:', err);
      this.error.set('Failed to update project name');
    } finally {
      this.loading.set(false);
    }
  }

  formatDate(timestamp: Timestamp | undefined): string {
    try {
      if (!timestamp) {
        return '';
      }
      return timestampDate(timestamp).toLocaleDateString('en-US', {
        year: 'numeric',
        month: 'long',
        day: 'numeric',
      });
    } catch {
      return '';
    }
  }
}
