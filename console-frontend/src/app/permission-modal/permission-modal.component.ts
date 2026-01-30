import { Component, EventEmitter, Input, Output, OnChanges, SimpleChanges } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { ModalComponent } from '../modal/modal.component';

interface Permission {
  name?: string;
  object: string;
  role: string;
}

interface ObjectItem {
  name: string;
  type: 'cluster' | 'namespace' | 'storage' | 'network';
}

interface RoleItem {
  name: string;
  applicableTypes: ('cluster' | 'namespace' | 'storage' | 'network')[];
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

  allObjects: ObjectItem[] = [
    { name: 'cluster-1', type: 'cluster' },
    { name: 'cluster-2', type: 'cluster' },
    { name: 'cluster-3', type: 'cluster' },
    { name: 'namespace-1', type: 'namespace' },
    { name: 'namespace-2', type: 'namespace' },
    { name: 'namespace-3', type: 'namespace' },
    { name: 'storage-1', type: 'storage' },
    { name: 'storage-2', type: 'storage' },
    { name: 'network-1', type: 'network' },
    { name: 'network-2', type: 'network' },
  ];

  allRoles: RoleItem[] = [
    { name: 'Cluster administrator', applicableTypes: ['cluster'] },
    { name: 'Storage owner', applicableTypes: ['storage'] },
    { name: 'Pod reader', applicableTypes: ['namespace'] },
    { name: 'Secret reader', applicableTypes: ['namespace'] },
    { name: 'Configmap updater', applicableTypes: ['namespace'] },
    { name: 'Deployment editor', applicableTypes: ['namespace'] },
    { name: 'Service viewer', applicableTypes: ['namespace'] },
    { name: 'Ingress administrator', applicableTypes: ['network'] },
    { name: 'Volume manager', applicableTypes: ['storage'] },
    { name: 'Network policy editor', applicableTypes: ['network'] },
    { name: 'Pod executor', applicableTypes: ['namespace'] },
  ];

  selectedUser = '';
  selectedObject = '';
  selectedRole = '';

  // Filtered lists based on selection
  get availableObjects(): ObjectItem[] {
    // Always show all objects - users can select any object
    return this.allObjects;
  }

  get availableRoles(): RoleItem[] {
    if (!this.selectedObject) {
      return this.allRoles;
    }
    const object = this.allObjects.find((obj) => obj.name === this.selectedObject);
    if (!object) {
      return this.allRoles;
    }
    return this.allRoles.filter((role) => role.applicableTypes.includes(object.type));
  }

  ngOnChanges(changes: SimpleChanges): void {
    if (this.show) {
      if (this.isEditMode && this.permission) {
        this.selectedUser = this.permission.name || '';
        this.selectedObject = this.permission.object;
        this.selectedRole = this.permission.role;
      } else {
        this.selectedUser = '';
        this.selectedObject = '';
        this.selectedRole = '';
      }
    }

    // Reset selections if they're no longer valid after filtering
    if (changes['show'] && this.show) {
      this.validateSelections();
    }
  }

  validateSelections(): void {
    // Only clear role if it's not applicable to the selected object
    if (this.selectedRole && !this.availableRoles.find((role) => role.name === this.selectedRole)) {
      this.selectedRole = '';
    }
  }

  onObjectChange(): void {
    // When object changes, check if current role is still applicable
    this.validateSelections();
  }

  onRoleChange(): void {
    // No validation needed when role changes since objects are always available
  }

  onClose(): void {
    this.closeModal.emit();
  }

  onSave(): void {
    const permission: Permission = {
      object: this.selectedObject,
      role: this.selectedRole,
    };

    if (!this.isEditMode) {
      permission.name = this.selectedUser;
    }

    this.save.emit(permission);
  }

  isFormValid(): boolean {
    if (this.isEditMode) {
      return !!this.selectedObject && !!this.selectedRole;
    }
    return !!this.selectedUser && !!this.selectedObject && !!this.selectedRole;
  }
}
