import { Component, inject, signal, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { ActivatedRoute } from '@angular/router';
import { TitleService } from '../title.service';
import { BreadcrumbComponent, BreadcrumbSegment } from '../breadcrumb/breadcrumb.component';
import { OrganizationDataService } from '../organization-data.service';
import { PermissionModalComponent } from '../permission-modal/permission-modal.component';
import { NgIcon, provideIcons } from '@ng-icons/core';
import { tablerPlus, tablerPencil, tablerTrash } from '@ng-icons/tabler-icons';

interface Permission {
  name: string;
  namespace: string;
  role: string;
}

@Component({
  selector: 'app-namespace-members',
  standalone: true,
  imports: [CommonModule, BreadcrumbComponent, PermissionModalComponent, NgIcon],
  viewProviders: [
    provideIcons({
      tablerPlus,
      tablerPencil,
      tablerTrash,
    }),
  ],
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

  // Permissions data for the namespace
  permissions: Permission[] = [
    { name: 'Alice Johnson', namespace: 'namespace-1', role: 'Pod reader' },
    { name: 'Bob Johnson', namespace: 'namespace-2', role: 'Pod reader' },
    { name: 'Charlie Brown', namespace: 'namespace-1', role: 'Secret reader' },
    { name: 'Emma Davis', namespace: 'namespace-3', role: 'Deployment editor' },
    { name: 'Grace Lee', namespace: 'namespace-2', role: 'Service viewer' },
    { name: 'Jack Robinson', namespace: 'namespace-3', role: 'Pod executor' },
  ];

  showModal = false;
  isEditMode = false;
  selectedPermission: Permission | null = null;
  editingIndex = -1;

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

  onSavePermission(permission: { name?: string; namespace: string; role: string }): void {
    if (this.isEditMode && this.editingIndex >= 0) {
      // Update existing permission
      this.permissions[this.editingIndex] = {
        ...this.permissions[this.editingIndex],
        namespace: permission.namespace,
        role: permission.role,
      };
    } else {
      // Add new permission
      if (permission.name) {
        this.permissions.push({
          name: permission.name,
          namespace: permission.namespace,
          role: permission.role,
        });
      }
    }
    this.closeModal();
  }
}
