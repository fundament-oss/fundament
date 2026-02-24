import { Component, inject, signal, OnInit, ChangeDetectionStrategy } from '@angular/core';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { ReactiveFormsModule, FormBuilder, Validators } from '@angular/forms';
import { firstValueFrom } from 'rxjs';
import { timestampDate } from '@bufbuild/protobuf/wkt';
import { NgIcon, provideIcons } from '@ng-icons/core';
import {
  tablerPlus,
  tablerTrash,
  tablerPencil,
  tablerAlertTriangle,
  tablerInfoCircle,
  tablerLock,
  tablerArrowBackUp,
} from '@ng-icons/tabler-icons';
import { TitleService } from '../title.service';
import { PROJECT, MEMBER } from '../../connect/tokens';
import ModalComponent from '../modal/modal.component';
import { formatTimeAgo } from '../utils/date-format';
import type { ProjectMember } from '../../generated/v1/project_pb';
import { ProjectMemberRole } from '../../generated/v1/project_pb';

interface ProjectMemberView {
  member: ProjectMember;
  source: 'org' | 'project';
  orgPermission: ProjectMemberRole | null;
}

const roleToString = (role: ProjectMemberRole): string => {
  switch (role) {
    case ProjectMemberRole.ADMIN:
      return 'admin';
    case ProjectMemberRole.VIEWER:
      return 'viewer';
    default:
      return 'unknown';
  }
};

const stringToRole = (s: string): ProjectMemberRole => {
  switch (s) {
    case 'admin':
      return ProjectMemberRole.ADMIN;
    default:
      return ProjectMemberRole.VIEWER;
  }
};

const formatMemberDate = (member: ProjectMember): string =>
  formatTimeAgo(member.created ? timestampDate(member.created) : undefined);

@Component({
  selector: 'app-project-members',
  imports: [ReactiveFormsModule, NgIcon, ModalComponent, RouterLink],
  viewProviders: [
    provideIcons({
      tablerPlus,
      tablerTrash,
      tablerPencil,
      tablerAlertTriangle,
      tablerInfoCircle,
      tablerLock,
      tablerArrowBackUp,
    }),
  ],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './project-members.component.html',
})
export default class ProjectMembersComponent implements OnInit {
  private titleService = inject(TitleService);

  private route = inject(ActivatedRoute);

  private fb = inject(FormBuilder);

  private projectClient = inject(PROJECT);

  private memberClient = inject(MEMBER);

  projectId = signal<string>('');

  memberViews = signal<ProjectMemberView[]>([]);

  availableUsers = signal<{ id: string; name: string }[]>([]);

  isLoading = signal(true);

  error = signal<string | null>(null);

  showAddMemberModal = signal<boolean>(false);

  isAddingMember = signal<boolean>(false);

  showRemoveMemberModal = signal<boolean>(false);

  pendingRemoveView = signal<ProjectMemberView | null>(null);

  editingMemberView = signal<ProjectMemberView | null>(null);

  memberForm = this.fb.group({
    userId: ['', Validators.required],
    permission: ['viewer', Validators.required],
  });

  ProjectMemberRole = ProjectMemberRole;

  constructor() {
    this.titleService.setTitle('Project members');
  }

  ngOnInit() {
    const projectId = this.route.snapshot.params['id'];
    this.projectId.set(projectId);
    this.loadMembers();
  }

  async loadMembers() {
    this.isLoading.set(true);
    this.error.set(null);

    try {
      const [projectResponse, orgResponse] = await Promise.all([
        firstValueFrom(this.projectClient.listProjectMembers({ projectId: this.projectId() })),
        firstValueFrom(this.memberClient.listMembers({})),
      ]);

      // Build a map of org member id → org role string
      const orgRoleByUserId = new Map<string, string>();
      orgResponse.members
        .filter((m) => m.externalRef)
        .forEach((m) => orgRoleByUserId.set(m.id, m.permission));

      // Enrich project members with source info
      const views: ProjectMemberView[] = projectResponse.members.map((member) => {
        const orgRole = orgRoleByUserId.get(member.userId);
        if (orgRole !== undefined) {
          const orgRoleEnum = stringToRole(orgRole);
          const sameRole = member.role === orgRoleEnum;
          return {
            member,
            source: sameRole ? 'org' : 'project',
            orgPermission: orgRoleEnum,
          };
        }
        return { member, source: 'project', orgPermission: null };
      });
      this.memberViews.set(views);

      // Available users for "add member" dropdown: org members not yet in project
      const projectUserIds = new Set(projectResponse.members.map((m) => m.userId));
      this.availableUsers.set(
        orgResponse.members
          .filter((m) => m.externalRef && !projectUserIds.has(m.userId))
          .map((m) => ({ id: m.userId, name: m.name })),
      );
    } catch (err) {
      this.error.set(
        err instanceof Error ? `Failed to load members: ${err.message}` : 'Failed to load members',
      );
    } finally {
      this.isLoading.set(false);
    }
  }

  openAddMemberModal() {
    this.editingMemberView.set(null);
    this.memberForm.reset({ userId: '', permission: 'viewer' });
    this.showAddMemberModal.set(true);
  }

  openEditMemberModal(view: ProjectMemberView) {
    this.editingMemberView.set(view);
    this.memberForm.patchValue({
      userId: view.member.userId,
      permission: roleToString(view.member.role),
    });
    this.showAddMemberModal.set(true);
  }

  async saveMember() {
    if (this.memberForm.invalid) {
      this.memberForm.markAllAsTouched();
      return;
    }

    this.isAddingMember.set(true);
    const role = stringToRole(this.memberForm.value.permission!);

    try {
      if (this.editingMemberView()) {
        await firstValueFrom(
          this.projectClient.updateProjectMemberRole({
            memberId: this.editingMemberView()!.member.id,
            role,
          }),
        );
      } else {
        const userId = this.memberForm.value.userId!;
        await firstValueFrom(
          this.projectClient.addProjectMember({
            projectId: this.projectId(),
            userId,
            role,
          }),
        );
      }

      this.showAddMemberModal.set(false);
      this.editingMemberView.set(null);
      await this.loadMembers();
    } catch (err) {
      this.showAddMemberModal.set(false);
      this.editingMemberView.set(null);
      this.error.set(
        err instanceof Error ? `Failed to save member: ${err.message}` : 'Failed to save member',
      );
    } finally {
      this.isAddingMember.set(false);
    }
  }

  openRemoveMemberModal(view: ProjectMemberView) {
    this.pendingRemoveView.set(view);
    this.showRemoveMemberModal.set(true);
  }

  async confirmRemoveMember() {
    const view = this.pendingRemoveView();
    if (!view) return;

    try {
      await firstValueFrom(this.projectClient.removeProjectMember({ memberId: view.member.id }));
      this.showRemoveMemberModal.set(false);
      await this.loadMembers();
    } catch (err) {
      this.error.set(
        err instanceof Error
          ? `Failed to remove member: ${err.message}`
          : 'Failed to remove member',
      );
      this.showRemoveMemberModal.set(false);
    }
  }

  async resetToOrgDefault(view: ProjectMemberView) {
    if (view.orgPermission === null) return;

    try {
      await firstValueFrom(
        this.projectClient.updateProjectMemberRole({
          memberId: view.member.id,
          role: view.orgPermission,
        }),
      );
      await this.loadMembers();
    } catch (err) {
      this.error.set(
        err instanceof Error
          ? `Failed to reset permission: ${err.message}`
          : 'Failed to reset permission',
      );
    }
  }

  roleToString = roleToString;

  formatMemberDate = formatMemberDate;
}
