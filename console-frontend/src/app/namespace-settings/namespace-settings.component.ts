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
  ListProjectNamespacesRequestSchema,
  type ProjectNamespace,
} from '../../generated/v1/project_pb';
import { firstValueFrom } from 'rxjs';
import { timestampDate, type Timestamp } from '@bufbuild/protobuf/wkt';

@Component({
  selector: 'app-namespace-settings',
  standalone: true,
  imports: [CommonModule, FormsModule, NgIconComponent, BreadcrumbComponent],
  viewProviders: [
    provideIcons({
      tablerX,
      tablerPencil,
      tablerCheck,
    }),
  ],
  templateUrl: './namespace-settings.component.html',
})
export class NamespaceSettingsComponent implements OnInit {
  private titleService = inject(TitleService);
  private route = inject(ActivatedRoute);
  private projectClient = inject(PROJECT);

  @ViewChild('nameInput') nameInput?: ElementRef<HTMLInputElement>;

  projectId = signal<string>('');
  namespaceId = signal<string>('');
  projectName = signal<string>('');
  namespace = signal<ProjectNamespace | undefined>(undefined);

  isEditing = signal(false);
  editingName = signal('');
  loading = signal(false);
  error = signal<string | null>(null);

  constructor() {
    this.titleService.setTitle('Namespace Settings');
  }

  async ngOnInit() {
    const id = this.route.snapshot.params['id'];
    const nsId = this.route.snapshot.params['namespaceId'];
    if (id && nsId) {
      this.projectId.set(id);
      this.namespaceId.set(nsId);
      await this.loadNamespaceAndProject(id, nsId);
    }
  }

  private async loadNamespaceAndProject(projectId: string, namespaceId: string) {
    this.loading.set(true);
    this.error.set(null);
    try {
      // Fetch project info for breadcrumb
      const projectRequest = create(GetProjectRequestSchema, { projectId });
      const projectResponse = await firstValueFrom(this.projectClient.getProject(projectRequest));
      if (projectResponse.project) {
        this.projectName.set(projectResponse.project.name);
      }

      // Fetch namespaces for this project
      const namespacesRequest = create(ListProjectNamespacesRequestSchema, { projectId });
      const namespacesResponse = await firstValueFrom(
        this.projectClient.listProjectNamespaces(namespacesRequest),
      );

      // Find the specific namespace
      const namespace = namespacesResponse.namespaces.find((ns) => ns.id === namespaceId);
      if (namespace) {
        this.namespace.set(namespace);
      } else {
        this.error.set('Namespace not found');
      }
    } catch (err) {
      console.error('Error loading namespace:', err);
      this.error.set('Failed to load namespace');
    } finally {
      this.loading.set(false);
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

    const currentNamespace = this.namespace();
    if (currentNamespace?.name) {
      segments.push({
        label: currentNamespace.name,
        route: `/projects/${this.projectId()}/namespaces/${this.namespaceId()}`,
      });
    }

    segments.push({ label: 'Settings' });

    return segments;
  }

  startEdit() {
    const currentNamespace = this.namespace();
    if (currentNamespace) {
      this.isEditing.set(true);
      this.editingName.set(currentNamespace.name);

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
    const currentNamespace = this.namespace();
    const nameToSave = this.editingName();

    if (!nameToSave.trim() || !currentNamespace) {
      return;
    }

    this.loading.set(true);
    this.error.set(null);

    try {
      // TODO: Update namespace API endpoint not yet implemented
      // For now, show an error message
      this.error.set('Namespace renaming is not yet supported');
      this.isEditing.set(false);
      this.editingName.set('');
    } catch (err) {
      console.error('Error updating namespace:', err);
      this.error.set('Failed to update namespace name');
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
