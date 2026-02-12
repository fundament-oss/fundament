import {
  Component,
  inject,
  signal,
  OnInit,
  ChangeDetectionStrategy,
  computed,
} from '@angular/core';
import { ActivatedRoute, RouterLink, RouterLinkActive } from '@angular/router';
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
import { ToastService } from '../toast.service';
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
  imports: [FormsModule, NgIcon, ModalComponent, RouterLink, RouterLinkActive],
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

  private toastService = inject(ToastService);

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

  modalNamespace = signal<string>('');

  modalRoles = signal<Record<string, boolean>>({});

  // Members available for role bindings (same users from project-members mock data)
  members = signal([
    { id: 'user-1', name: 'Alice Johnson' },
    { id: 'user-2', name: 'Bob Smith' },
    { id: 'user-3', name: 'Carol Williams' },
    { id: 'user-5', name: 'Eve Davis' },
  ]);

  namespaces = signal(['production', 'staging', 'development']);

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
        namespace: 'production',
        roles: ['deploy', 'view-pods'],
      },
      {
        id: 'rb-2',
        userId: 'user-2',
        memberName: 'Bob Smith',
        namespace: 'staging',
        roles: ['deploy', 'view-pods', 'view-logs'],
      },
      {
        id: 'rb-3',
        userId: 'user-5',
        memberName: 'Eve Davis',
        namespace: 'staging',
        roles: ['view-pods'],
      },
      {
        id: 'rb-4',
        userId: 'user-5',
        memberName: 'Eve Davis',
        namespace: 'development',
        roles: ['deploy', 'view-pods', 'view-logs'],
      },
    ]);
  }

  openCreateModal() {
    this.editingBinding.set(null);
    this.modalMemberId.set('');
    this.modalNamespace.set('');
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

  saveBinding() {
    const memberId = this.modalMemberId();
    const namespace = this.modalNamespace();
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
      this.toastService.success(`Role binding updated for ${member.name}`);
    } else {
      const newBinding: RoleBinding = {
        id: `rb-${Date.now()}`,
        userId: memberId,
        memberName: member.name,
        namespace,
        roles: selectedRoles,
      };
      this.roleBindings.update((bindings) => [...bindings, newBinding]);
      this.toastService.success(`Role binding created for ${member.name} in ${namespace}`);
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
    this.toastService.info(
      `Role binding removed for ${binding.memberName} in ${binding.namespace}`,
    );
    this.showRemoveModal.set(false);
  }
}
