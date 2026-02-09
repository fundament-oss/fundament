import { Component, inject, signal, OnInit, ChangeDetectionStrategy } from '@angular/core';
import { CommonModule } from '@angular/common';
import { ActivatedRoute } from '@angular/router';
import { ReactiveFormsModule, FormBuilder, Validators } from '@angular/forms';
import { NgIcon, provideIcons } from '@ng-icons/core';
import { tablerPlus, tablerTrash, tablerPencil } from '@ng-icons/tabler-icons';
import { TitleService } from '../title.service';
import { ToastService } from '../toast.service';
import ModalComponent from '../modal/modal.component';

type ProjectMemberRole = 'viewer' | 'admin';

interface ProjectMember {
  id: string;
  userId: string;
  name: string;
  email: string;
  role: ProjectMemberRole;
  addedAt: string;
}

@Component({
  selector: 'app-project-members',
  imports: [CommonModule, ReactiveFormsModule, NgIcon, ModalComponent],
  viewProviders: [
    provideIcons({
      tablerPlus,
      tablerTrash,
      tablerPencil,
    }),
  ],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './project-members.component.html',
})
export default class ProjectMembersComponent implements OnInit {
  private titleService = inject(TitleService);

  private route = inject(ActivatedRoute);

  private fb = inject(FormBuilder);

  private toastService = inject(ToastService);

  projectId = signal<string>('');

  members = signal<ProjectMember[]>([]);

  availableUsers = signal<{ id: string; name: string; email: string }[]>([]);

  showAddMemberModal = signal<boolean>(false);

  isAddingMember = signal<boolean>(false);

  editingMember = signal<ProjectMember | null>(null);

  memberForm = this.fb.group({
    userId: ['', Validators.required],
    role: ['viewer' as ProjectMemberRole, Validators.required],
  });

  constructor() {
    this.titleService.setTitle('Project members');
  }

  ngOnInit() {
    const projectId = this.route.snapshot.params['id'];
    this.projectId.set(projectId);
    this.loadMembers();
  }

  loadMembers() {
    // Mock data for project members
    this.members.set([
      {
        id: 'pm-1',
        userId: 'user-1',
        name: 'Alice Johnson',
        email: 'alice.johnson@example.com',
        role: 'admin',
        addedAt: '2024-01-15T10:30:00Z',
      },
      {
        id: 'pm-2',
        userId: 'user-2',
        name: 'Bob Smith',
        email: 'bob.smith@example.com',
        role: 'viewer',
        addedAt: '2024-02-20T14:45:00Z',
      },
      {
        id: 'pm-3',
        userId: 'user-3',
        name: 'Carol Williams',
        email: 'carol.williams@example.com',
        role: 'viewer',
        addedAt: '2024-03-10T09:15:00Z',
      },
    ]);

    // Mock available users (users not yet in the project)
    this.availableUsers.set([
      { id: 'user-4', name: 'David Brown', email: 'david.brown@example.com' },
      { id: 'user-5', name: 'Eve Davis', email: 'eve.davis@example.com' },
      { id: 'user-6', name: 'Frank Miller', email: 'frank.miller@example.com' },
    ]);
  }

  openAddMemberModal() {
    this.editingMember.set(null);
    this.memberForm.reset({ userId: '', role: 'viewer' });
    this.showAddMemberModal.set(true);
  }

  openEditMemberModal(member: ProjectMember) {
    this.editingMember.set(member);
    this.memberForm.patchValue({ userId: member.userId, role: member.role });
    this.showAddMemberModal.set(true);
  }

  saveMember() {
    if (this.memberForm.invalid) {
      this.memberForm.markAllAsTouched();
      return;
    }

    this.isAddingMember.set(true);
    const role = this.memberForm.value.role as ProjectMemberRole;

    if (this.editingMember()) {
      // Edit existing member
      const member = this.editingMember()!;
      this.members.update((members) =>
        members.map((m) => (m.id === member.id ? { ...m, role } : m)),
      );
      this.toastService.success(
        `${member.name}'s role updated to ${role === 'admin' ? 'Project admin' : 'Project member'}`,
      );
    } else {
      // Add new member
      const userId = this.memberForm.value.userId!;
      const user = this.availableUsers().find((u) => u.id === userId);

      if (user) {
        const newMember: ProjectMember = {
          id: `pm-${Date.now()}`,
          userId: user.id,
          name: user.name,
          email: user.email,
          role,
          addedAt: new Date().toISOString(),
        };

        this.members.update((members) => [...members, newMember]);
        this.availableUsers.update((users) => users.filter((u) => u.id !== userId));
        this.toastService.success(`${user.name} added to project`);
      }
    }

    this.showAddMemberModal.set(false);
    this.isAddingMember.set(false);
    this.editingMember.set(null);
  }

  removeMember(memberId: string) {
    const member = this.members().find((m) => m.id === memberId);
    if (!member) return;

    // eslint-disable-next-line no-alert
    if (!window.confirm(`Are you sure you want to remove ${member.name} from this project?`)) {
      return;
    }

    // Move user back to available users
    this.availableUsers.update((users) => [
      ...users,
      { id: member.userId, name: member.name, email: member.email },
    ]);
    this.members.update((members) => members.filter((m) => m.id !== memberId));
    this.toastService.info(`${member.name} removed from project`);
  }
}
