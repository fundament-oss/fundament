import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, Router } from '@angular/router';
import { of } from 'rxjs';
import { vi } from 'vitest';
import { create } from '@bufbuild/protobuf';
import OrganizationMemberSheetComponent from './organization-member-sheet.component';
import { MEMBER } from '../../connect/tokens';
import { NotificationService } from '../notification.service';
import { OrganizationDataService } from '../organization-data.service';
import AuthnApiService from '../authn-api.service';
import { MemberSchema, type Member } from '../../generated/v1/member_pb';

const ALICE = create(MemberSchema, {
  id: 'membership-alice',
  userId: '019b4000-1000-7000-8000-00000000a11c',
  name: 'Alice',
  email: 'alice@example.com',
  externalRef: 'ext-alice',
  permission: 'viewer',
  status: 'accepted',
});

const BOB_INVITED = create(MemberSchema, {
  id: 'membership-bob',
  userId: '019b4000-1000-7000-8000-000000000b0b',
  name: 'bob@example.com',
  email: 'bob@example.com',
  permission: 'viewer',
  status: 'pending',
});

const CAROL_DECLINED = create(MemberSchema, {
  id: 'membership-carol',
  userId: '019b4000-1000-7000-8000-0000000ca201',
  name: 'carol@example.com',
  email: 'carol@example.com',
  permission: 'viewer',
  status: 'declined',
});

const ME = create(MemberSchema, {
  id: 'membership-me',
  userId: 'user-me',
  name: 'Me',
  email: 'me@example.com',
  externalRef: 'ext-me',
  permission: 'admin',
  status: 'accepted',
});

async function build(
  memberId: string,
  members: Member[] = [ALICE, BOB_INVITED, CAROL_DECLINED, ME],
  { listFails = false } = {},
) {
  const notifications = { success: vi.fn(), error: vi.fn() };
  const memberClient = {
    updateMemberPermission: vi.fn(() => of({})),
    deleteMember: vi.fn(() => of({})),
  };
  const router = { navigate: vi.fn() };
  const membersSignal = signal(members);
  const organizationData = {
    members: membersSignal,
    reloadMembers: vi.fn(async () => {
      if (listFails) throw new Error('unreachable');
      return membersSignal();
    }),
    forgetMember: vi.fn((id: string) =>
      membersSignal.update((list) => list.filter((m) => m.id !== id)),
    ),
    currentOrganizationDisplayName: signal('Acme'),
  };

  TestBed.configureTestingModule({
    imports: [OrganizationMemberSheetComponent],
    providers: [
      { provide: ActivatedRoute, useValue: { snapshot: { params: { memberId } } } },
      { provide: Router, useValue: router },
      { provide: MEMBER, useValue: memberClient },
      { provide: NotificationService, useValue: notifications },
      { provide: OrganizationDataService, useValue: organizationData },
      { provide: AuthnApiService, useValue: { currentUser$: of({ id: 'user-me' }) } },
    ],
  });

  const fixture = TestBed.createComponent(OrganizationMemberSheetComponent);
  fixture.detectChanges();
  await fixture.whenStable();
  fixture.detectChanges();
  return { fixture, notifications, memberClient, router, organizationData };
}

/** The sheet portals itself to the body (SheetSyncDirective), so it is looked
 *  up there rather than under the fixture. */
const sheet = (): HTMLElement =>
  document.querySelector('nldd-sheet[accessible-label="Organization member"]') as HTMLElement;

const text = (): string => sheet()?.textContent ?? '';

const find = (selector: string): Element | null => sheet()?.querySelector(selector) ?? null;

describe('OrganizationMemberSheetComponent', () => {
  const original = Object.getOwnPropertyDescriptor(navigator, 'clipboard');

  afterEach(() => {
    if (original) {
      Object.defineProperty(navigator, 'clipboard', original);
    } else {
      delete (navigator as { clipboard?: Clipboard }).clipboard;
    }
  });

  it('shows a member by membership ID, with their user ID and email address', async () => {
    const { fixture } = await build('membership-alice');

    expect(fixture.componentInstance.member()?.id).toBe('membership-alice');
    expect(text()).toContain('019b4000-1000-7000-8000-00000000a11c');
    expect(find('[accessible-label="Copy user ID"]')).not.toBeNull();
    expect(find('[accessible-label="Copy email address"]')).not.toBeNull();
    expect(fixture.componentInstance.dateLabel()).toBe('Joined');
    expect(text()).toContain('Remove Alice from organization');
  });

  // Declining sets the status without soft-deleting the row, so ListMembers
  // still returns it; the list hides it, and so must a link to it.
  it('treats a declined invitation as not found', async () => {
    const { fixture } = await build('membership-carol');

    expect(fixture.componentInstance.member()).toBeNull();
    expect(text()).not.toContain('Remove');
    expect(find('nldd-inline-dialog')?.getAttribute('text')).toBe('Member not found');
  });

  it('names an invitation by its address and offers to cancel it', async () => {
    const { fixture, memberClient, notifications, router } = await build('membership-bob');

    expect(fixture.componentInstance.displayName()).toBe('bob@example.com');
    expect(fixture.componentInstance.dateLabel()).toBe('Invited');
    expect(text()).toContain('Cancel invitation for bob@example.com');

    await fixture.componentInstance.confirmRemove();

    expect(memberClient.deleteMember).toHaveBeenCalledWith({ id: 'membership-bob' });
    expect(notifications.success).toHaveBeenCalledWith(
      "Invitation for 'bob@example.com' cancelled",
    );
    expect(router.navigate).toHaveBeenCalled();
  });

  it('says the list did not load, not that the member is gone, when the cache is empty', async () => {
    const { fixture, organizationData } = await build('membership-alice', [], {
      listFails: true,
    });

    const dialog = find('nldd-inline-dialog');
    expect(dialog?.getAttribute('text')).toBe('Member not loaded');
    expect(dialog?.getAttribute('supporting-text')).toBe('unreachable');
    expect(find('nldd-button[text="Try again"]')).not.toBeNull();

    await fixture.componentInstance.load();
    expect(organizationData.reloadMembers).toHaveBeenCalledTimes(2);
  });

  it('still shows a member the cache holds when the reload fails', async () => {
    const { fixture } = await build('membership-alice', undefined, { listFails: true });

    expect(fixture.componentInstance.member()?.id).toBe('membership-alice');
    expect(text()).toContain('Remove Alice from organization');
  });

  it('drops a removed member from the cache when the reload after it fails', async () => {
    const { fixture, organizationData, router } = await build('membership-alice', undefined, {
      listFails: true,
    });

    await fixture.componentInstance.confirmRemove();

    expect(organizationData.forgetMember).toHaveBeenCalledWith('membership-alice');
    expect(organizationData.members().map((m) => m.id)).not.toContain('membership-alice');
    expect(router.navigate).toHaveBeenCalled();
  });

  it('offers no actions on your own membership', async () => {
    const { fixture } = await build('membership-me');

    expect(fixture.componentInstance.isCurrentUser()).toBe(true);
    expect(text()).not.toContain('Remove');
    expect(find('[accessible-label="Role actions"]')).toBeNull();
    // Copying is not an action on the membership: you can copy your own ID.
    expect(find('[accessible-label="Copy user ID"]')).not.toBeNull();
  });

  it('leaves out the email row when the identity provider sent no address', async () => {
    const noEmail = create(MemberSchema, {
      id: ALICE.id,
      userId: ALICE.userId,
      name: ALICE.name,
      externalRef: ALICE.externalRef,
      permission: ALICE.permission,
      status: ALICE.status,
    });
    await build('membership-alice', [noEmail]);

    expect(find('[accessible-label="Copy user ID"]')).not.toBeNull();
    expect(find('[accessible-label="Copy email address"]')).toBeNull();
  });

  it('copies the user ID, not the membership ID, and says so', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true });
    const { fixture, notifications } = await build('membership-alice');

    await fixture.componentInstance.copyUserId(ALICE);

    expect(writeText).toHaveBeenCalledWith('019b4000-1000-7000-8000-00000000a11c');
    expect(notifications.success).toHaveBeenCalledWith('User ID copied');
  });

  it('reports a copy that failed instead of claiming it worked', async () => {
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText: vi.fn().mockRejectedValue(new Error('denied')) },
      configurable: true,
    });
    const { fixture, notifications } = await build('membership-alice');

    await fixture.componentInstance.copyEmail(ALICE);

    expect(notifications.success).not.toHaveBeenCalled();
    expect(notifications.error).toHaveBeenCalledWith(
      'Email address not copied',
      expect.any(String),
    );
  });
});
