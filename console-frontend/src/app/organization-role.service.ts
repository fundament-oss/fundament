import { Injectable, computed, inject } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import AuthnApiService from './authn-api.service';
import { OrganizationDataService } from './organization-data.service';

export type OrganizationRole = 'admin' | 'viewer';

/**
 * What you are in the organization you are reading: its admin or one of its
 * viewers. Read from the member list the organization loads with anyway, so it
 * costs no call of its own, and it follows an organization switch because that
 * list does.
 *
 * It scopes the navigation, not access: the backend decides what you may do,
 * this only decides what the menu offers.
 */
@Injectable({ providedIn: 'root' })
export default class OrganizationRoleService {
  private organizationData = inject(OrganizationDataService);

  private currentUser = toSignal(inject(AuthnApiService).currentUser$);

  /** Whether the answer is in: the member list has loaded and someone is
   *  signed in. Until then neither admin nor viewer holds, so nothing appears
   *  that later has to go again and nothing is refused that later turns out
   *  to be allowed. */
  roleKnown = computed(
    () => this.organizationData.membersLoaded() && this.currentUser() !== undefined,
  );

  /** Your role here, or null while it is not known or when you are not an
   *  accepted member. A pending invitation is not a role yet. */
  role = computed<OrganizationRole | null>(() => {
    if (!this.roleKnown()) return null;
    const userId = this.currentUser()?.id;
    const member = this.organizationData
      .members()
      .find((m) => m.userId === userId && m.status === 'accepted');
    if (member?.permission === 'admin') return 'admin';
    if (member?.permission === 'viewer') return 'viewer';
    return null;
  });

  isOrgAdmin = computed(() => this.role() === 'admin');

  /** Known not to be an admin. Not the opposite of isOrgAdmin: while the role
   *  is unknown both are false. */
  isOrgViewer = computed(() => this.roleKnown() && !this.isOrgAdmin());
}
