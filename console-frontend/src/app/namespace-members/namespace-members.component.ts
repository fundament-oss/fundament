import { Component, inject, signal, OnInit, ChangeDetectionStrategy } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute } from '@angular/router';
import { TitleService } from '../title.service';
import { BreadcrumbComponent, BreadcrumbSegment } from '../breadcrumb/breadcrumb.component';
import { OrganizationDataService } from '../organization-data.service';
import { ModalComponent } from '../modal/modal.component';
import { NgIcon, provideIcons } from '@ng-icons/core';
import { tablerPlus, tablerPencil, tablerTrash } from '@ng-icons/tabler-icons';

interface Permission {
  name: string;
  namespace: string;
  role: string;
}

interface NamespaceItem {
  name: string;
  type: 'namespace';
}

interface RoleItem {
  name: string;
  applicableTypes: 'namespace'[];
}

@Component({
  selector: 'app-namespace-members',
  imports: [CommonModule, FormsModule, BreadcrumbComponent, ModalComponent, NgIcon],
  viewProviders: [
    provideIcons({
      tablerPlus,
      tablerPencil,
      tablerTrash,
    }),
  ],
  templateUrl: './namespace-members.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
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

  // Available options for selects
  users = ['John Doe', 'Jane Smith', 'Alice Johnson', 'Bob Johnson', 'Charlie Brown'];

  allNamespaces: NamespaceItem[] = [
    { name: 'namespace-1', type: 'namespace' },
    { name: 'namespace-2', type: 'namespace' },
    { name: 'namespace-3', type: 'namespace' },
  ];

  allRoles: RoleItem[] = [
    { name: 'Pod reader', applicableTypes: ['namespace'] },
    { name: 'Secret reader', applicableTypes: ['namespace'] },
    { name: 'Configmap updater', applicableTypes: ['namespace'] },
    { name: 'Deployment editor', applicableTypes: ['namespace'] },
    { name: 'Service viewer', applicableTypes: ['namespace'] },
    { name: 'Pod executor', applicableTypes: ['namespace'] },
  ];

  selectedUser = '';
  selectedNamespace = '';
  selectedRole = '';

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

  get availableNamespaces(): NamespaceItem[] {
    return this.allNamespaces;
  }

  get availableRoles(): RoleItem[] {
    return this.allRoles;
  }

  onAddPermission(): void {
    this.isEditMode = false;
    this.selectedPermission = null;
    this.selectedUser = '';
    this.selectedNamespace = '';
    this.selectedRole = '';
    this.showModal = true;
  }

  onEditPermission(permission: Permission, index: number): void {
    this.isEditMode = true;
    this.selectedPermission = permission;
    this.selectedUser = permission.name;
    this.selectedNamespace = permission.namespace;
    this.selectedRole = permission.role;
    this.editingIndex = index;
    this.showModal = true;
  }

  closeModal(): void {
    this.showModal = false;
    this.selectedPermission = null;
    this.editingIndex = -1;
  }

  savePermission(): void {
    if (!this.isFormValid()) {
      return;
    }

    if (this.isEditMode && this.editingIndex >= 0) {
      // Update existing permission
      this.permissions[this.editingIndex] = {
        ...this.permissions[this.editingIndex],
        namespace: this.selectedNamespace,
        role: this.selectedRole,
      };
    } else {
      // Add new permission
      this.permissions.push({
        name: this.selectedUser,
        namespace: this.selectedNamespace,
        role: this.selectedRole,
      });
    }
    this.closeModal();
  }

  isFormValid(): boolean {
    if (this.isEditMode) {
      return !!this.selectedNamespace && !!this.selectedRole;
    }
    return !!this.selectedUser && !!this.selectedNamespace && !!this.selectedRole;
  }

  removePermission(index: number): void {
    const permission = this.permissions[index];
    if (!permission) return;

    if (!confirm(`Are you sure you want to remove permission for ${permission.name}?`)) {
      return;
    }

    this.permissions.splice(index, 1);
  }
}
