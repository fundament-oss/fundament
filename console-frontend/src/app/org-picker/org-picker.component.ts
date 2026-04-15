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

  selectOrganization = output<string>();

  acceptInvitation = output<Invitation>();

  declineInvitation = output<Invitation>();

  private el = inject(ElementRef<HTMLElement>);

  private pendingOrgIds = computed(() => new Set(this.invitations().map((i) => i.organizationId)));

  acceptedOrganizations = computed(() => {
    const pending = this.pendingOrgIds();
    return this.organizations().filter((org) => !pending.has(org.id));
  });

  pendingInvitationList = computed(() => this.invitations());

  constructor() {
    afterNextRender(() => {
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
}
