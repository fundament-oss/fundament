import { Component, EventEmitter, Input, Output, OnChanges, SimpleChanges } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { ModalComponent } from '../modal/modal.component';

interface Permission {
  name?: string;
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
  selector: 'app-permission-modal',
  standalone: true,
  imports: [CommonModule, FormsModule, ModalComponent],
  templateUrl: './permission-modal.component.html',
})
export class PermissionModalComponent implements OnChanges {
  @Input() show = false;
  @Input() isEditMode = false;
  @Input() permission: Permission | null = null;

  @Output() closeModal = new EventEmitter<void>();
  @Output() save = new EventEmitter<Permission>();

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

  // Filtered lists based on selection
  get availableNamespaces(): NamespaceItem[] {
    return this.allNamespaces;
  }

  get availableRoles(): RoleItem[] {
    return this.allRoles;
  }

  ngOnChanges(changes: SimpleChanges): void {
    if (this.show) {
      if (this.isEditMode && this.permission) {
        this.selectedUser = this.permission.name || '';
        this.selectedNamespace = this.permission.namespace;
        this.selectedRole = this.permission.role;
      } else {
        this.selectedUser = '';
        this.selectedNamespace = '';
        this.selectedRole = '';
      }
    }
  }

  onNamespaceChange(): void {
    // No validation needed since all roles are namespace-applicable
  }

  onRoleChange(): void {
    // No validation needed since all roles are namespace-applicable
  }

  onClose(): void {
    this.closeModal.emit();
  }

  onSave(): void {
    const permission: Permission = {
      namespace: this.selectedNamespace,
      role: this.selectedRole,
    };

    if (!this.isEditMode) {
      permission.name = this.selectedUser;
    }

    this.save.emit(permission);
  }

  isFormValid(): boolean {
    if (this.isEditMode) {
      return !!this.selectedNamespace && !!this.selectedRole;
    }
    return !!this.selectedUser && !!this.selectedNamespace && !!this.selectedRole;
  }
}
