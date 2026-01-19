import { Component, inject, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { TitleService } from '../title.service';
import { PlusIconComponent, CloseIconComponent, TrashIconComponent } from '../icons';
import {
  ButtonComponent,
  CardComponent,
  ModalComponent,
  BadgeComponent,
  EmptyStateComponent,
} from '../shared/components';

interface OrganizationMember {
  id: string;
  name: string;
  email: string;
  role: 'admin' | 'viewer';
  isCurrentUser?: boolean;
}

interface PendingInvitation {
  id: string;
  email: string;
  role: 'admin' | 'viewer';
  invitedAt: Date;
}

@Component({
  selector: 'app-organization-members',
  imports: [
    CommonModule,
    FormsModule,
    PlusIconComponent,
    CloseIconComponent,
    TrashIconComponent,
    ButtonComponent,
    CardComponent,
    ModalComponent,
    BadgeComponent,
    EmptyStateComponent,
  ],
  templateUrl: './organization-members.component.html',
})
export class OrganizationMembersComponent {
  private titleService = inject(TitleService);

  // Modal state
  isModalOpen = signal(false);
  inviteEmail = signal('');
  inviteRole = signal<'admin' | 'viewer'>('viewer');

  // Mock data - replace with API calls later
  members = signal<OrganizationMember[]>([
    {
      id: '1',
      name: 'John Doe',
      email: 'john.doe@example.com',
      role: 'admin',
      isCurrentUser: true,
    },
    { id: '2', name: 'Jane Smith', email: 'jane.smith@example.com', role: 'viewer' },
    { id: '3', name: 'Bob Williams', email: 'bob.williams@example.com', role: 'admin' },
  ]);

  pendingInvitations = signal<PendingInvitation[]>([
    {
      id: '1',
      email: 'newuser@example.com',
      role: 'viewer',
      invitedAt: new Date(Date.now() - 2 * 24 * 60 * 60 * 1000),
    },
    {
      id: '2',
      email: 'another.person@company.com',
      role: 'admin',
      invitedAt: new Date(Date.now() - 5 * 24 * 60 * 60 * 1000),
    },
  ]);

  constructor() {
    this.titleService.setTitle('Organization members');
  }

  openModal() {
    this.inviteEmail.set('');
    this.inviteRole.set('viewer');
    this.isModalOpen.set(true);
  }

  closeModal() {
    this.isModalOpen.set(false);
  }

  submitInvitation() {
    const email = this.inviteEmail();
    const role = this.inviteRole();

    if (!email.trim()) {
      return;
    }

    // Add to pending invitations (mock - replace with API call)
    this.pendingInvitations.update((invitations) => [
      ...invitations,
      { id: crypto.randomUUID(), email: email.trim(), role, invitedAt: new Date() },
    ]);

    this.closeModal();
  }

  cancelInvitation(id: string) {
    this.pendingInvitations.update((invitations) => invitations.filter((inv) => inv.id !== id));
  }

  updateMemberRole(memberId: string, role: 'admin' | 'viewer') {
    this.members.update((members) => members.map((m) => (m.id === memberId ? { ...m, role } : m)));
  }

  removeMember(id: string) {
    this.members.update((members) => members.filter((m) => m.id !== id));
  }

  formatTimeAgo(date: Date): string {
    const now = new Date();
    const diffMs = now.getTime() - date.getTime();
    const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));

    if (diffDays === 0) {
      return 'today';
    } else if (diffDays === 1) {
      return 'yesterday';
    } else {
      return `${diffDays} days ago`;
    }
  }

  getInitials(name: string): string {
    return name
      .split(' ')
      .map((n) => n[0])
      .join('')
      .toUpperCase()
      .slice(0, 2);
  }

  getAvatarColor(name: string): string {
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
  }
}
