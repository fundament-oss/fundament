import {
  Component,
  inject,
  signal,
  OnInit,
  ChangeDetectionStrategy,
  computed,
} from '@angular/core';
import { ActivatedRoute } from '@angular/router';
import { FormsModule } from '@angular/forms';
import { NgIcon, provideIcons } from '@ng-icons/core';
import {
  tablerPlus,
  tablerTrash,
  tablerPencil,
  tablerInfoCircle,
  tablerAlertTriangle,
} from '@ng-icons/tabler-icons';
import { TitleService } from '../title.service';
import ModalComponent from '../modal/modal.component';

interface RoleBinding {
  id: string;
  userId: string;
  memberName: string;
  namespace: string;
  roles: string[];
}

const AVAILABLE_ROLES = ['deploy', 'view-pods', 'view-logs', 'manage-services'];

@Component({
  selector: 'app-project-roles',
  imports: [FormsModule, NgIcon, ModalComponent],
  viewProviders: [
    provideIcons({
      tablerPlus,
      tablerTrash,
      tablerPencil,
      tablerInfoCircle,
      tablerAlertTriangle,
    }),
  ],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './project-roles.component.html',
})
export default class ProjectRolesComponent implements OnInit {
  private titleService = inject(TitleService);

  private route = inject(ActivatedRoute);

  projectId = signal<string>('');

  roleBindings = signal<RoleBinding[]>([]);

  availableRoles = AVAILABLE_ROLES;

  // Filter state
  namespaceFilter = signal<string>('');

  // Modal state
  showCreateModal = signal<boolean>(false);

  editingBinding = signal<RoleBinding | null>(null);

  isSubmitting = signal<boolean>(false);

  // Remove modal state
  showRemoveModal = signal<boolean>(false);

  pendingBindingId = signal<string>('');

  pendingBindingDescription = computed(() => {
    const binding = this.roleBindings().find((rb) => rb.id === this.pendingBindingId());
    return binding ? `${binding.memberName} in ${binding.namespace}` : '';
  });

  // Modal form fields
  modalMemberId = signal<string>('');

  namespaceMode = signal<'all' | 'specific' | 'custom'>('all');

  modalNamespace = signal<string>('');

  modalCustomNamespace = signal<string>('');

  modalRoles = signal<Record<string, boolean>>({});

  // Members available for role bindings (same users from project-members mock data)
  members = signal([
    { id: 'user-1', name: 'Alice Johnson' },
    { id: 'user-2', name: 'Bob Smith' },
    { id: 'user-3', name: 'Carol Williams' },
    { id: 'user-5', name: 'Eve Davis' },
  ]);

  namespaces = signal(['namespace-1', 'namespace-2', 'namespace-3']);

  uniqueNamespaces = computed(() => {
    const ns = new Set(this.roleBindings().map((rb) => rb.namespace));
    return [...ns].sort();
  });

  filteredBindings = computed(() => {
    const ns = this.namespaceFilter();
    if (ns) {
      return this.roleBindings().filter((rb) => rb.namespace === ns);
    }
    return this.roleBindings();
  });

  constructor() {
    this.titleService.setTitle('Project roles');
  }

  ngOnInit() {
    const projectId = this.route.snapshot.params['id'];
    this.projectId.set(projectId);
    this.loadRoleBindings();
  }

  loadRoleBindings() {
    this.roleBindings.set([
      {
        id: 'rb-1',
        userId: 'user-2',
        memberName: 'Bob Smith',
        namespace: 'namespace-1',
        roles: ['deploy', 'view-pods'],
      },
      {
        id: 'rb-2',
        userId: 'user-2',
        memberName: 'Bob Smith',
        namespace: 'namespace-2',
        roles: ['deploy', 'view-pods', 'view-logs'],
      },
      {
        id: 'rb-3',
        userId: 'user-5',
        memberName: 'Eve Davis',
        namespace: 'namespace-2',
        roles: ['view-pods'],
      },
      {
        id: 'rb-4',
        userId: 'user-5',
        memberName: 'Eve Davis',
        namespace: 'namespace-3',
        roles: ['deploy', 'view-pods', 'view-logs'],
      },
    ]);
  }

  openCreateModal() {
    this.editingBinding.set(null);
    this.modalMemberId.set('');
    this.namespaceMode.set('all');
    this.modalNamespace.set('');
    this.modalCustomNamespace.set('');
    this.modalRoles.set(Object.fromEntries(AVAILABLE_ROLES.map((r) => [r, false])));
    this.showCreateModal.set(true);
  }

  openEditModal(binding: RoleBinding) {
    this.editingBinding.set(binding);
    this.modalMemberId.set(binding.userId);
    this.modalNamespace.set(binding.namespace);
    this.modalRoles.set(
      Object.fromEntries(AVAILABLE_ROLES.map((r) => [r, binding.roles.includes(r)])),
    );
    this.showCreateModal.set(true);
  }

  toggleRole(role: string) {
    this.modalRoles.update((roles) => ({ ...roles, [role]: !roles[role] }));
  }

  onNamespaceModeChange(mode: 'all' | 'specific' | 'custom') {
    this.namespaceMode.set(mode);
    if (mode === 'specific') {
      setTimeout(() => document.getElementById('rb-namespace')?.focus());
    } else if (mode === 'custom') {
      setTimeout(() => document.getElementById('rb-custom-namespace')?.focus());
    }
  }

  resolveNamespace(): string {
    switch (this.namespaceMode()) {
      case 'all':
        return '*';
      case 'specific':
        return this.modalNamespace();
      case 'custom':
        return this.modalCustomNamespace().trim();
      default:
        throw new Error(`unexpected namespace mode: ${this.namespaceMode()}`);
    }
  }

  saveBinding() {
    const memberId = this.modalMemberId();
    const namespace = this.editingBinding()
      ? this.editingBinding()!.namespace
      : this.resolveNamespace();
    const selectedRoles = Object.entries(this.modalRoles())
      .filter(([, selected]) => selected)
      .map(([role]) => role);

    if (!memberId || !namespace || selectedRoles.length === 0) {
      return;
    }

    this.isSubmitting.set(true);

    const member = this.members().find((m) => m.id === memberId);
    if (!member) return;

    if (this.editingBinding()) {
      const editId = this.editingBinding()!.id;
      this.roleBindings.update((bindings) =>
        bindings.map((rb) => (rb.id === editId ? { ...rb, roles: selectedRoles } : rb)),
      );
    } else {
      const newBinding: RoleBinding = {
        id: `rb-${Date.now()}`,
        userId: memberId,
        memberName: member.name,
        namespace,
        roles: selectedRoles,
      };
      this.roleBindings.update((bindings) => [...bindings, newBinding]);
    }

    this.showCreateModal.set(false);
    this.isSubmitting.set(false);
    this.editingBinding.set(null);
  }

  openRemoveModal(bindingId: string) {
    this.pendingBindingId.set(bindingId);
    this.showRemoveModal.set(true);
  }

  confirmRemoveBinding() {
    const bindingId = this.pendingBindingId();
    const binding = this.roleBindings().find((rb) => rb.id === bindingId);
    if (!binding) return;

    this.roleBindings.update((bindings) => bindings.filter((rb) => rb.id !== bindingId));
    this.showRemoveModal.set(false);
  }
}
