import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, Router } from '@angular/router';
import { of, throwError } from 'rxjs';
import { vi } from 'vitest';
import { create } from '@bufbuild/protobuf';
import ProjectMemberSheetComponent from './project-member-sheet.component';
import { PROJECT } from '../../connect/tokens';
import { NotificationService } from '../notification.service';
import { OrganizationDataService } from '../organization-data.service';
import PageNavService from '../page-nav.service';
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
];

const ALICE = create(ProjectMemberSchema, {
  id: 'pm-alice',
  projectId: 'project-1',
  userId: 'user-alice',
  userName: 'Alice',
  role: ProjectMemberRole.VIEWER,
});

async function build(memberId: string, { listFails = false } = {}) {
  const notifications = { success: vi.fn(), error: vi.fn() };
  const router = { navigate: vi.fn() };
  const projectClient = {
    listProjectMembers: vi.fn(() =>
      listFails ? throwError(() => new Error('unreachable')) : of({ members: [ALICE] }),
    ),
    updateProjectMemberRole: vi.fn(() => of({})),
    removeProjectMember: vi.fn(() => of({})),
  };
  const organizationData = {
    membersChanged: signal(0),
    reloadMembers: vi.fn(async () => ORG_MEMBERS),
    getProjectById: () => undefined,
  };

  TestBed.configureTestingModule({
    imports: [ProjectMemberSheetComponent],
    providers: [
      {
        provide: ActivatedRoute,
        useValue: { snapshot: { params: { memberId }, parent: { params: { id: 'project-1' } } } },
      },
      { provide: Router, useValue: router },
      { provide: PROJECT, useValue: projectClient },
      { provide: NotificationService, useValue: notifications },
      { provide: OrganizationDataService, useValue: organizationData },
      {
        provide: PageNavService,
        useValue: { path: (p: string) => p, goTo: vi.fn(), backText: () => null },
      },
    ],
  });

  const fixture = TestBed.createComponent(ProjectMemberSheetComponent);
  fixture.detectChanges();
  await vi.waitFor(() => expect(fixture.componentInstance.isLoading()).toBe(false));
  fixture.detectChanges();
  return { fixture, notifications, router, projectClient, organizationData };
}

/** The sheet portals itself to the body (SheetSyncDirective), so it is looked
 *  up there rather than under the fixture. */
const find = (selector: string): Element | null =>
  document
    .querySelector('nldd-sheet[accessible-label="Project member"]')
    ?.querySelector(selector) ?? null;

describe('ProjectMemberSheetComponent', () => {
  it('says the member did not load, not that they are gone, when loading fails', async () => {
    const { fixture, projectClient } = await build('pm-alice', { listFails: true });

    const dialog = find('nldd-inline-dialog');
    expect(dialog?.getAttribute('text')).toBe('Member not loaded');
    expect(dialog?.getAttribute('supporting-text')).toBe('unreachable');
    expect(find('nldd-button[text="Try again"]')).not.toBeNull();

    await fixture.componentInstance.load();
    expect(projectClient.listProjectMembers).toHaveBeenCalledTimes(2);
  });

  it('says not found only when the list loaded without the member', async () => {
    const { fixture } = await build('pm-nobody');

    expect(fixture.componentInstance.member()).toBeNull();
    expect(find('nldd-inline-dialog')?.getAttribute('text')).toBe('Member not found');
  });

  it('keeps showing the member when the reload after a role change fails', async () => {
    const { fixture, projectClient } = await build('pm-alice');
    const view = fixture.componentInstance.member();
    expect(view).not.toBeNull();

    projectClient.listProjectMembers.mockReturnValue(throwError(() => new Error('unreachable')));
    await fixture.componentInstance.togglePermission(view!);

    expect(fixture.componentInstance.member()?.member.id).toBe('pm-alice');
    expect(fixture.componentInstance.loadError()).toBe('unreachable');
  });

  // The list is the parent route and stays mounted behind the sheet: without
  // the bump it would go on showing the old role, or the removed member.
  it('tells the list behind it about a role change', async () => {
    const { fixture, projectClient, organizationData } = await build('pm-alice');

    await fixture.componentInstance.togglePermission(fixture.componentInstance.member()!);

    expect(projectClient.updateProjectMemberRole).toHaveBeenCalledWith({
      memberId: 'pm-alice',
      role: ProjectMemberRole.ADMIN,
    });
    expect(organizationData.membersChanged()).toBe(1);
  });

  it('tells the list behind it about a removal, then closes', async () => {
    const { fixture, projectClient, organizationData, router } = await build('pm-alice');

    await fixture.componentInstance.confirmRemove();

    expect(projectClient.removeProjectMember).toHaveBeenCalledWith({ memberId: 'pm-alice' });
    expect(organizationData.membersChanged()).toBe(1);
    expect(router.navigate).toHaveBeenCalled();
  });

  it('tells the list nothing when the removal fails', async () => {
    const { fixture, projectClient, organizationData, router } = await build('pm-alice');
    projectClient.removeProjectMember.mockReturnValue(throwError(() => new Error('denied')));

    await fixture.componentInstance.confirmRemove();

    expect(organizationData.membersChanged()).toBe(0);
    expect(router.navigate).not.toHaveBeenCalled();
  });
});
