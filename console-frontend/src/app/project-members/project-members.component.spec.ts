import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, provideRouter } from '@angular/router';
import { of } from 'rxjs';
import { vi } from 'vitest';
import { create } from '@bufbuild/protobuf';
import ProjectMembersComponent from './project-members.component';
import { PROJECT } from '../../connect/tokens';
import { NotificationService } from '../notification.service';
import { OrganizationDataService } from '../organization-data.service';
import PageNavService from '../page-nav.service';
import { TitleService } from '../title.service';
import { MemberSchema } from '../../generated/v1/member_pb';
import { ProjectMemberRole, ProjectMemberSchema } from '../../generated/v1/project_pb';

const ORG_MEMBERS = [
  create(MemberSchema, {
    id: 'membership-alice',
    userId: 'user-alice',
    name: 'Alice',
    email: 'alice@example.com',
    externalRef: 'ext-alice',
    permission: 'viewer',
    status: 'accepted',
  }),
  // Signed in, but the identity provider sent no address.
  create(MemberSchema, {
    id: 'membership-dave',
    userId: 'user-dave',
    name: 'Dave',
    externalRef: 'ext-dave',
    permission: 'viewer',
    status: 'accepted',
  }),
];

const PROJECT_MEMBERS = [
  create(ProjectMemberSchema, {
    id: 'pm-alice',
    projectId: 'project-1',
    userId: 'user-alice',
    userName: 'Alice',
    role: ProjectMemberRole.VIEWER,
  }),
  create(ProjectMemberSchema, {
    id: 'pm-dave',
    projectId: 'project-1',
    userId: 'user-dave',
    userName: 'Dave',
    role: ProjectMemberRole.VIEWER,
  }),
];

async function build() {
  const notifications = { success: vi.fn(), error: vi.fn() };
  const listProjectMembers = vi.fn(() => of({ members: PROJECT_MEMBERS }));
  const organizationData = {
    membersChanged: signal(0),
    reloadMembers: vi.fn(async () => ORG_MEMBERS),
    getProjectById: () => undefined,
  };

  TestBed.configureTestingModule({
    imports: [ProjectMembersComponent],
    providers: [
      provideRouter([]),
      { provide: ActivatedRoute, useValue: { snapshot: { params: { id: 'project-1' } } } },
      {
        provide: PROJECT,
        useValue: { listProjectMembers },
      },
      { provide: NotificationService, useValue: notifications },
      {
        provide: PageNavService,
        useValue: { path: (p: string) => p, goTo: vi.fn(), backText: () => null },
      },
      { provide: TitleService, useValue: { setTitle: vi.fn() } },
      { provide: OrganizationDataService, useValue: organizationData },
    ],
  });

  const fixture = TestBed.createComponent(ProjectMembersComponent);
  fixture.detectChanges();
  // Both lists are fetched together, and the rows render once both are in.
  await vi.waitFor(() => expect(fixture.componentInstance.isLoading()).toBe(false));
  fixture.detectChanges();
  return {
    fixture,
    notifications,
    listProjectMembers,
    organizationData,
    root: fixture.nativeElement as HTMLElement,
  };
}

const menuFor = (root: HTMLElement, projectMemberId: string): HTMLElement | null =>
  root.querySelector(`nldd-menu[anchor="project-member-actions-${projectMemberId}"]`);

const itemTexts = (menu: HTMLElement | null): string[] =>
  Array.from(menu?.querySelectorAll('nldd-menu-item') ?? []).map(
    (item) => item.getAttribute('text') ?? '',
  );

describe('ProjectMembersComponent rows', () => {
  it('splits each row into a link to the sheet and a menu button, without a chevron', async () => {
    const { root } = await build();

    const row = root.querySelector('nldd-list-item:has([id="project-member-actions-pm-alice"])');
    expect(row?.hasAttribute('href')).toBe(false);
    const segments = row?.querySelectorAll('nldd-list-item-segment') ?? [];
    expect(segments.length).toBe(2);
    expect(segments[0].getAttribute('href')).toBe('/projects/project-1/members/pm-alice');
    expect(segments[1].getAttribute('accessible-label')).toBe('Actions for Alice');
    expect(row?.querySelector('nldd-icon-cell[icon="chevron-right"]')).toBeNull();
    expect(root.querySelector('nldd-list nldd-menu')).toBeNull();
  });

  it('takes the email address from the organization member list', async () => {
    const { root } = await build();

    expect(itemTexts(menuFor(root, 'pm-alice'))).toEqual([
      'View details',
      'Copy user ID',
      'Copy email address',
    ]);
    expect(itemTexts(menuFor(root, 'pm-dave'))).toEqual(['View details', 'Copy user ID']);
  });

  it('copies the address it found there', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    const original = Object.getOwnPropertyDescriptor(navigator, 'clipboard');
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true });
    try {
      const { fixture, notifications } = await build();
      const view = fixture.componentInstance
        .visibleMembers()
        .find((v) => v.member.id === 'pm-alice');

      await fixture.componentInstance.copyEmail(view!);

      expect(writeText).toHaveBeenCalledWith('alice@example.com');
      expect(notifications.success).toHaveBeenCalledWith('Email address copied');
    } finally {
      if (original) {
        Object.defineProperty(navigator, 'clipboard', original);
      } else {
        delete (navigator as { clipboard?: Clipboard }).clipboard;
      }
    }
  });

  // The member sheet is a child route, so this list stays mounted behind it
  // and only learns about a change through membersChanged.
  it('reloads when a sheet reports a changed or removed member', async () => {
    const { fixture, listProjectMembers, organizationData } = await build();
    expect(listProjectMembers).toHaveBeenCalledTimes(1);

    organizationData.membersChanged.update((count) => count + 1);
    TestBed.tick();
    await vi.waitFor(() => expect(fixture.componentInstance.isLoading()).toBe(false));

    expect(listProjectMembers).toHaveBeenCalledTimes(2);
  });
});
