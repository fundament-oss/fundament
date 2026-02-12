import { Component, inject, OnInit, signal, ChangeDetectionStrategy } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { firstValueFrom } from 'rxjs';
import { ConnectError, Code } from '@connectrpc/connect';
import { timestampDate } from '@bufbuild/protobuf/wkt';
import { NgIcon, provideIcons } from '@ng-icons/core';
import {
  tablerPlus,
  tablerTrash,
  tablerClockHour4,
  tablerMail,
  tablerAlertTriangle,
  tablerX,
  tablerInfoCircle,
  tablerPencil,
} from '@ng-icons/tabler-icons';
import { heroUserGroup } from '@ng-icons/heroicons/outline';
import { TitleService } from '../title.service';
import AuthnApiService from '../authn-api.service';
import { MEMBER } from '../../connect/tokens';
import ModalComponent from '../modal/modal.component';

const formatTimeAgo = (date: Date | undefined): string => {
  if (!date) {
    return '';
  }

  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));

  if (diffDays === 0) {
    return 'today';
  }
  if (diffDays === 1) {
    return 'yesterday';
  }
  return `${diffDays} days ago`;
};

const getInitials = (name: string): string =>
  name
    .split(' ')
    .map((n) => n[0])
    .join('')
    .toUpperCase()
    .slice(0, 2);

const getAvatarColor = (name: string): string => {
  const colors = [
    'bg-indigo-600',
    'bg-emerald-600',
    'bg-purple-600',
    'bg-rose-600',
    'bg-amber-600',
    'bg-cyan-600',
  ];
  const index = name.charCodeAt(0) % colors.length;
  return colors[index];
};

interface OrganizationMember {
  id: string;
  name: string;
  email?: string;
  externalId?: string;
  permission: string;
  isCurrentUser?: boolean;
  isPending: boolean;
  created?: Date;
}

@Component({
  selector: 'app-organization-members',
  imports: [CommonModule, FormsModule, NgIcon, ModalComponent],
  viewProviders: [
    provideIcons({
      tablerPlus,
      tablerX,
      tablerTrash,
      tablerClockHour4,
      tablerMail,
      tablerAlertTriangle,
      tablerInfoCircle,
      tablerPencil,
      heroUserGroup,
    }),
  ],
  templateUrl: './organization-members.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class OrganizationMembersComponent implements OnInit {
  private titleService = inject(TitleService);

  private memberClient = inject(MEMBER);

  private authnService = inject(AuthnApiService);

  // Loading and error state
  isLoading = signal(true);

  error = signal<string | null>(null);

  isSubmitting = signal(false);

  // Invite modal state
  isModalOpen = signal(false);

  inviteEmail = signal('');

  invitePermission = signal('viewer');

  inviteError = signal<string | null>(null);

  // Delete modal state
  showDeleteModal = signal(false);

  deletingMember = signal<OrganizationMember | null>(null);

  // Edit modal state
  showEditModal = signal(false);

  editingMember = signal<OrganizationMember | null>(null);

  editPermission = signal('viewer');

  isUpdating = signal(false);

  // All members loaded from API (includes both active and pending)
  allMembers = signal<OrganizationMember[]>([]);

  // Computed: active members (have external_id)
  get activeMembers(): OrganizationMember[] {
    return this.allMembers().filter((m) => !m.isPending);
  }

  // Computed: pending invitations (no external_id)
  get pendingInvitations(): OrganizationMember[] {
    return this.allMembers().filter((m) => m.isPending);
  }

  constructor() {
    this.titleService.setTitle('Organization members');
  }

  ngOnInit() {
    this.loadMembers();
  }

  async loadMembers() {
    this.isLoading.set(true);
    this.error.set(null);

    try {
      const currentUser = await firstValueFrom(this.authnService.currentUser$);
      const response = await firstValueFrom(this.memberClient.listMembers({}));

      const members: OrganizationMember[] = response.members.map((member) => ({
        id: member.id,
        name: member.name,
        email: member.email,
        externalId: member.externalId,
        permission: member.role,
        isCurrentUser: currentUser?.id === member.id,
        isPending: !member.externalId,
        created: member.created ? timestampDate(member.created) : undefined,
      }));

      this.allMembers.set(members);
    } catch (err) {
      this.error.set(
        err instanceof Error ? `Failed to load members: ${err.message}` : 'Failed to load members',
      );
    } finally {
      this.isLoading.set(false);
    }
  }

  openModal() {
    this.inviteEmail.set('');
    this.invitePermission.set('viewer');
    this.inviteError.set(null);
    this.isModalOpen.set(true);
  }

  closeModal() {
    this.isModalOpen.set(false);
  }

  async submitInvitation() {
    const email = this.inviteEmail().trim();

    if (!email) {
      return;
    }

    this.isSubmitting.set(true);
    this.inviteError.set(null);

    try {
      await firstValueFrom(
        this.memberClient.inviteMember({ email, role: this.invitePermission() }),
      );
      this.closeModal();
      await this.loadMembers();
    } catch (err: unknown) {
      if (err instanceof ConnectError) {
        if (err.code === Code.AlreadyExists) {
          this.inviteError.set('This email address is already in use.');
        } else if (err.code === Code.InvalidArgument) {
          this.inviteError.set('Please enter a valid email address.');
        } else {
          this.inviteError.set('Failed to invite member. Please try again.');
        }
      } else {
        this.inviteError.set('Failed to invite member. Please try again.');
      }
    } finally {
      this.isSubmitting.set(false);
    }
  }

  async cancelInvitation(id: string) {
    try {
      await firstValueFrom(this.memberClient.deleteMember({ id }));
      await this.loadMembers();
    } catch (err) {
      this.error.set(
        err instanceof Error
          ? `Failed to cancel invitation: ${err.message}`
          : 'Failed to cancel invitation',
      );
    }
  }

  openDeleteModal(member: OrganizationMember) {
    this.deletingMember.set(member);
    this.showDeleteModal.set(true);
  }

  async confirmDeleteMember() {
    const member = this.deletingMember();
    if (!member) return;

    try {
      await firstValueFrom(this.memberClient.deleteMember({ id: member.id }));
      this.showDeleteModal.set(false);
      this.deletingMember.set(null);
      await this.loadMembers();
    } catch (err) {
      this.error.set(
        err instanceof Error
          ? `Failed to remove member: ${err.message}`
          : 'Failed to remove member',
      );
      this.showDeleteModal.set(false);
    }
  }

  openEditModal(member: OrganizationMember) {
    this.editingMember.set(member);
    this.editPermission.set(member.permission);
    this.showEditModal.set(true);
  }

  async confirmEditMember() {
    const member = this.editingMember();
    if (!member) return;

    const newPermission = this.editPermission();
    if (newPermission === member.permission) {
      this.showEditModal.set(false);
      return;
    }

    this.isUpdating.set(true);

    try {
      // Re-invite with the new role (delete + invite)
      await firstValueFrom(this.memberClient.deleteMember({ id: member.id }));
      const email = member.email || member.name;
      await firstValueFrom(this.memberClient.inviteMember({ email, role: newPermission }));
      this.showEditModal.set(false);
      this.editingMember.set(null);
      await this.loadMembers();
    } catch (err) {
      this.error.set(
        err instanceof Error
          ? `Failed to update member: ${err.message}`
          : 'Failed to update member',
      );
      this.showEditModal.set(false);
    } finally {
      this.isUpdating.set(false);
    }
  }

  formatTimeAgo = formatTimeAgo;

  getInitials = getInitials;

  getAvatarColor = getAvatarColor;
}
