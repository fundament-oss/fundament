import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { of } from 'rxjs';
import { vi } from 'vitest';
import { create } from '@bufbuild/protobuf';
import OrganizationMembersComponent from './organization-members.component';
import { MEMBER, INVITE } from '../../connect/tokens';
import { NotificationService } from '../notification.service';
import { OrganizationDataService } from '../organization-data.service';
import AuthnApiService from '../authn-api.service';
import PageNavService from '../page-nav.service';
import { TitleService } from '../title.service';
import { MemberSchema, type Member } from '../../generated/v1/member_pb';

const ALICE = create(MemberSchema, {
  id: 'membership-alice',
  userId: 'user-alice',
  name: 'Alice',
  email: 'alice@example.com',
  externalRef: 'ext-alice',
  permission: 'viewer',
  status: 'accepted',
});

const BOB_INVITED = create(MemberSchema, {
  id: 'membership-bob',
  userId: 'user-bob',
  name: 'bob@example.com',
  email: 'bob@example.com',
  permission: 'viewer',
  status: 'pending',
});

const ME = create(MemberSchema, {
  id: 'membership-me',
  userId: 'user-me',
  name: 'Me',
  externalRef: 'ext-me',
  permission: 'admin',
  status: 'accepted',
});

async function build(members: Member[] = [ALICE, BOB_INVITED, ME]) {
  const membersSignal = signal(members);
  const pageNav = { path: (p: string) => p, goTo: vi.fn(), backText: () => null };
  const notifications = { success: vi.fn(), error: vi.fn() };

  TestBed.configureTestingModule({
    imports: [OrganizationMembersComponent],
    providers: [
      provideRouter([]),
      { provide: MEMBER, useValue: {} },
      { provide: INVITE, useValue: {} },
      { provide: NotificationService, useValue: notifications },
      { provide: PageNavService, useValue: pageNav },
      { provide: TitleService, useValue: { setTitle: vi.fn() } },
      {
        provide: OrganizationDataService,
        useValue: {
          members: membersSignal,
          reloadCurrentMembers: vi.fn(async () => membersSignal()),
          currentOrganizationDisplayName: signal('Acme'),
        },
      },
      { provide: AuthnApiService, useValue: { currentUser$: of({ id: 'user-me' }) } },
    ],
  });

  const fixture = TestBed.createComponent(OrganizationMembersComponent);
  fixture.detectChanges();
  await fixture.whenStable();
  fixture.detectChanges();
  return { fixture, pageNav, notifications, root: fixture.nativeElement as HTMLElement };
}

/** The menu that hangs off a row's ellipsis segment. */
const menuFor = (root: HTMLElement, membershipId: string): HTMLElement | null =>
  root.querySelector(`nldd-menu[anchor="member-actions-${membershipId}"]`);

const itemTexts = (menu: HTMLElement | null): string[] =>
  Array.from(menu?.querySelectorAll('nldd-menu-item') ?? []).map(
    (item) => item.getAttribute('text') ?? '',
  );

describe('OrganizationMembersComponent rows', () => {
  it('splits each row into a link to the sheet and a menu button', async () => {
    const { root } = await build();

    const row = root.querySelector('nldd-list-item:has([id="member-actions-membership-alice"])');
    expect(row).not.toBeNull();
    // The row itself is no control: a control inside a control is invalid.
    expect(row?.hasAttribute('href')).toBe(false);
    const segments = row?.querySelectorAll('nldd-list-item-segment') ?? [];
    expect(segments.length).toBe(2);
    expect(segments[0].getAttribute('href')).toBe('/members/membership-alice');
    expect(segments[1].hasAttribute('button')).toBe(true);
    expect(segments[1].getAttribute('accessible-label')).toBe('Actions for Alice');
    expect(row?.querySelector('nldd-icon-cell[icon="chevron-right"]')).toBeNull();
  });

  it('anchors one menu per row after the list, never inside it', async () => {
    const { root } = await build();

    const list = root.querySelector('nldd-list[accessible-label="Members"]');
    expect(list?.querySelector('nldd-menu')).toBeNull();
    ['membership-alice', 'membership-bob', 'membership-me'].forEach((id) =>
      expect(menuFor(root, id)).not.toBeNull(),
    );
  });

  it('offers details, copying and the member actions on a member', async () => {
    const { root } = await build();

    expect(itemTexts(menuFor(root, 'membership-alice'))).toEqual([
      'View details',
      'Copy user ID',
      'Copy email address',
      'Make organization admin',
      'Remove',
    ]);
  });

  it('offers details, copying and the invitation actions on an invitation', async () => {
    const { root } = await build();

    expect(itemTexts(menuFor(root, 'membership-bob'))).toEqual([
      'View details',
      'Copy user ID',
      'Copy email address',
      'Switch to organization admin',
      'Send invitation email…',
      'Cancel invitation',
    ]);
  });

  it('offers only details and copying on your own row, and no address it does not have', async () => {
    const { root } = await build();

    expect(itemTexts(menuFor(root, 'membership-me'))).toEqual(['View details', 'Copy user ID']);
  });

  it('copies the user ID from the menu, not the membership ID', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    const original = Object.getOwnPropertyDescriptor(navigator, 'clipboard');
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true });
    try {
      const { root, notifications } = await build();

      menuFor(root, 'membership-alice')
        ?.querySelector('nldd-menu-item[text="Copy user ID"]')
        ?.dispatchEvent(new CustomEvent('select'));
      await Promise.resolve();

      expect(writeText).toHaveBeenCalledWith('user-alice');
      await vi.waitFor(() => expect(notifications.success).toHaveBeenCalledWith('User ID copied'));
    } finally {
      if (original) {
        Object.defineProperty(navigator, 'clipboard', original);
      } else {
        delete (navigator as { clipboard?: Clipboard }).clipboard;
      }
    }
  });
});
