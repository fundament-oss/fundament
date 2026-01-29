import { Component, inject, signal, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterLink, RouterLinkActive, ActivatedRoute } from '@angular/router';
import { TitleService } from '../title.service';
import { PermissionModalComponent } from '../permission-modal/permission-modal.component';
import { NgIcon, provideIcons } from '@ng-icons/core';
import { tablerPlus, tablerPencil, tablerTrash } from '@ng-icons/tabler-icons';

interface Permission {
  name: string;
  object: string;
  role: string;
}

@Component({
  selector: 'app-project-permissions',
  standalone: true,
  imports: [CommonModule, PermissionModalComponent, RouterLink, RouterLinkActive, NgIcon],
  viewProviders: [
    provideIcons({
      tablerPlus,
      tablerPencil,
      tablerTrash,
    }),
  ],
  templateUrl: './project-permissions.component.html',
})
export class ProjectPermissionsComponent implements OnInit {
  private titleService = inject(TitleService);
  private route = inject(ActivatedRoute);

  projectId = signal<string>('');

  // Permissions data for the project
  permissions: Permission[] = [
    { name: 'John Doe', object: 'cluster-1', role: 'Cluster administrator' },
    { name: 'Jane Smith', object: 'cluster-2', role: 'Cluster administrator' },
    { name: 'Alice Johnson', object: 'namespace-1', role: 'Storage owner' },
    { name: 'Bob Johnson', object: 'namespace-2', role: 'Pod reader' },
    { name: 'Charlie Brown', object: 'namespace-1', role: 'Secret reader' },
    { name: 'David Wilson', object: 'cluster-1', role: 'Configmap updater' },
    { name: 'Emma Davis', object: 'namespace-3', role: 'Deployment editor' },
    { name: 'Frank Miller', object: 'cluster-2', role: 'Service viewer' },
    { name: 'Grace Lee', object: 'namespace-2', role: 'Ingress administrator' },
    { name: 'Henry Adams', object: 'namespace-1', role: 'Volume manager' },
    { name: 'Iris Chen', object: 'cluster-1', role: 'Network policy editor' },
    { name: 'Jack Robinson', object: 'namespace-3', role: 'Pod executor' },
  ];

  showModal = false;
  isEditMode = false;
  selectedPermission: Permission | null = null;
  editingIndex = -1;

  constructor() {
    this.titleService.setTitle('Project permissions');
  }

  ngOnInit() {
    const id = this.route.snapshot.params['id'];
    if (id) {
      this.projectId.set(id);
    }
  }

  onAddPermission(): void {
    this.isEditMode = false;
    this.selectedPermission = null;
    this.showModal = true;
  }

  onEditPermission(permission: Permission, index: number): void {
    this.isEditMode = true;
    this.selectedPermission = permission;
    this.editingIndex = index;
    this.showModal = true;
  }

  closeModal(): void {
    this.showModal = false;
    this.selectedPermission = null;
    this.editingIndex = -1;
  }

  onSavePermission(permission: { name?: string; object: string; role: string }): void {
    if (this.isEditMode && this.editingIndex >= 0) {
      // Update existing permission
      this.permissions[this.editingIndex] = {
        ...this.permissions[this.editingIndex],
        object: permission.object,
        role: permission.role,
      };
    } else {
      // Add new permission
      if (permission.name) {
        this.permissions.push({
          name: permission.name,
          object: permission.object,
          role: permission.role,
        });
      }
    }
    this.closeModal();
  }
}
