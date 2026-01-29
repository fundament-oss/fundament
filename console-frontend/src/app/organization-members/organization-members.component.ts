import {
  Component,
  inject,
  OnInit,
  signal,
  ChangeDetectionStrategy,
  viewChild,
  ElementRef,
  afterNextRender,
  Injector,
} from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { firstValueFrom } from 'rxjs';
import { ConnectError, Code } from '@connectrpc/connect';
import { TitleService } from '../title.service';
import { NgIcon, provideIcons } from '@ng-icons/core';
import {
  tablerPlus,
  tablerX,
  tablerTrash,
  tablerClockHour4,
  tablerMail,
  tablerAlertTriangle,
} from '@ng-icons/tabler-icons';
import { heroUserGroup } from '@ng-icons/heroicons/outline';
import { AuthnApiService } from '../authn-api.service';
import { MEMBER } from '../../connect/tokens';
import { BreadcrumbComponent } from '../breadcrumb/breadcrumb.component';

interface OrganizationMember {
  id: string;
  name: string;
  email?: string;
  externalId?: string;
  role: string;
  isCurrentUser?: boolean;
  isPending: boolean;
  createdAt?: Date;
}

@Component({
  selector: 'app-organization-members',
  imports: [CommonModule, FormsModule, NgIcon, BreadcrumbComponent],
  viewProviders: [
    provideIcons({
      tablerPlus,
      tablerX,
      tablerTrash,
      tablerClockHour4,
      tablerMail,
      tablerAlertTriangle,
      heroUserGroup,
    }),
  ],
  templateUrl: './organization-members.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class OrganizationMembersComponent implements OnInit {
  private titleService = inject(TitleService);
  private memberClient = inject(MEMBER);
  private authnService = inject(AuthnApiService);
  private injector = inject(Injector);

  // Loading and error state
  isLoading = signal(true);
  error = signal<string | null>(null);
  isSubmitting = signal(false);

  // Modal state
  isModalOpen = signal(false);
  inviteEmail = signal('');
  inviteRole = signal('viewer');
  inviteError = signal<string | null>(null);
  private emailInput = viewChild<ElementRef<HTMLInputElement>>('emailInput');

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
        role: member.role,
        isCurrentUser: currentUser?.id === member.id,
        isPending: !member.externalId,
        createdAt: member.createdAt?.value ? new Date(member.createdAt.value) : undefined,
      }));

      this.allMembers.set(members);
    } catch (err) {
      this.error.set('Failed to load members. Please try again.');
      console.error('Failed to load members:', err);
    } finally {
      this.isLoading.set(false);
    }
  }

  openModal() {
    this.inviteEmail.set('');
    this.inviteRole.set('viewer');
    this.inviteError.set(null);
    this.isModalOpen.set(true);
    afterNextRender(
      () => {
        this.emailInput()?.nativeElement.focus();
      },
      { injector: this.injector },
    );
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
      await firstValueFrom(this.memberClient.inviteMember({ email, role: this.inviteRole() }));
      this.closeModal();
      await this.loadMembers();
    } catch (err: unknown) {
      console.error('Failed to invite member:', err);
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
      console.error('Failed to cancel invitation:', err);
      this.error.set('Failed to cancel invitation. Please try again.');
    }
  }

  formatTimeAgo(date: Date | undefined): string {
    if (!date) {
      return '';
    }

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
