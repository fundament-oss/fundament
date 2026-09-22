import {
  Component,
  input,
  output,
  computed,
  ChangeDetectionStrategy,
  afterNextRender,
  inject,
  ElementRef,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import type { Organization } from '../../generated/v1/organization_pb';
import type { Invitation } from '../../generated/v1/invite_pb';
import isPresenting from '../presentation/presenting';

import '@nldd/design-system/box';
import '@nldd/design-system/button';
import '@nldd/design-system/cell';
import '@nldd/design-system/container';
import '@nldd/design-system/icon-cell';
import '@nldd/design-system/list';
import '@nldd/design-system/list-item';
import '@nldd/design-system/page';
import '@nldd/design-system/rich-text';
import '@nldd/design-system/simple-section';
import '@nldd/design-system/spacer';
import '@nldd/design-system/spacer-cell';
import '@nldd/design-system/text-cell';
import '@nldd/design-system/title';

@Component({
  selector: 'app-org-picker',
  imports: [],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './org-picker.component.html',
})
export default class OrgPickerComponent {
  organizations = input<Organization[]>([]);

  invitations = input<Invitation[]>([]);

  /** Who is signed in, so someone in no organization can tell the operator
   *  which account to add. */
  userName = input<string>('');

  selectOrganization = output<string>();

  acceptInvitation = output<Invitation>();

  declineInvitation = output<Invitation>();

  /** Asks the shell to look again for organizations, for someone in none who
   *  has since been added to one by an operator. */
  recheck = output<void>();

  private el = inject(ElementRef<HTMLElement>);

  private pendingOrgIds = computed(() => new Set(this.invitations().map((i) => i.organizationId)));

  acceptedOrganizations = computed(() => {
    const pending = this.pendingOrgIds();
    return this.organizations().filter((org) => !pending.has(org.id));
  });

  pendingInvitationList = computed(() => this.invitations());

  /** Nothing to pick and nothing to answer: signing in creates no organization,
   *  so a first-time user lands here until an operator adds them to one. */
  hasNothingToChoose = computed(
    () => this.acceptedOrganizations().length === 0 && this.invitations().length === 0,
  );

  constructor() {
    afterNextRender(() => {
      if (isPresenting()) return;
      this.el.nativeElement.querySelector('button')?.focus();
    });
  }

  onSelect(orgId: string) {
    this.selectOrganization.emit(orgId);
  }

  onAccept(invitation: Invitation) {
    this.acceptInvitation.emit(invitation);
  }

  onDecline(invitation: Invitation) {
    this.declineInvitation.emit(invitation);
  }

  onRecheck() {
    this.recheck.emit();
  }
}
