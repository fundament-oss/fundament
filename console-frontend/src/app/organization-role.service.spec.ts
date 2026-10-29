import { TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { BehaviorSubject } from 'rxjs';
import { describe, expect, it } from 'vitest';
import OrganizationRoleService from './organization-role.service';
import AuthnApiService from './authn-api.service';
import { OrganizationDataService } from './organization-data.service';

interface FakeMember {
  userId: string;
  permission: string;
  status: string;
}

function roleFor(
  userId: string | undefined,
  members: FakeMember[],
  membersLoaded = true,
): OrganizationRoleService {
  TestBed.resetTestingModule();
  TestBed.configureTestingModule({
    providers: [
      {
        provide: AuthnApiService,
        useValue: { currentUser$: new BehaviorSubject(userId ? { id: userId } : undefined) },
      },
      {
        provide: OrganizationDataService,
        useValue: { members: signal(members), membersLoaded: signal(membersLoaded) },
      },
    ],
  });
  return TestBed.inject(OrganizationRoleService);
}

const members: FakeMember[] = [
  { userId: 'u-admin', permission: 'admin', status: 'accepted' },
  { userId: 'u-viewer', permission: 'viewer', status: 'accepted' },
  { userId: 'u-invited', permission: 'admin', status: 'pending' },
];

describe('OrganizationRoleService', () => {
  it('knows an accepted admin', () => {
    const role = roleFor('u-admin', members);
    expect(role.role()).toBe('admin');
    expect(role.isOrgAdmin()).toBe(true);
    expect(role.isOrgViewer()).toBe(false);
  });

  it('knows an accepted viewer', () => {
    const role = roleFor('u-viewer', members);
    expect(role.role()).toBe('viewer');
    expect(role.isOrgAdmin()).toBe(false);
    expect(role.isOrgViewer()).toBe(true);
  });

  it('does not count a pending invitation as admin', () => {
    const role = roleFor('u-invited', members);
    expect(role.role()).toBeNull();
    expect(role.isOrgAdmin()).toBe(false);
    expect(role.isOrgViewer()).toBe(true);
  });

  it('treats a user missing from the list as no admin', () => {
    const role = roleFor('u-stranger', members);
    expect(role.role()).toBeNull();
    expect(role.isOrgAdmin()).toBe(false);
  });

  it('claims nothing while the members are still loading', () => {
    const role = roleFor('u-admin', members, false);
    expect(role.roleKnown()).toBe(false);
    expect(role.isOrgAdmin()).toBe(false);
    expect(role.isOrgViewer()).toBe(false);
  });

  it('claims nothing while nobody is signed in', () => {
    const role = roleFor(undefined, members);
    expect(role.roleKnown()).toBe(false);
    expect(role.isOrgViewer()).toBe(false);
  });
});
