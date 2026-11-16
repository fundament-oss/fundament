import {
  Component,
  inject,
  input,
  signal,
  computed,
  effect,
  Output,
  EventEmitter,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { ReactiveFormsModule, FormBuilder, Validators } from '@angular/forms';
import { firstValueFrom } from 'rxjs';
import { createIdempotencyRef, withIdempotency } from '../../connect/idempotency';
import { NotificationService } from '../notification.service';
import { OrganizationDataService } from '../organization-data.service';
import { PROJECT } from '../../connect/tokens';
import SheetSyncDirective from '../sheet-sync.directive';
import AutofocusDirective from '../autofocus.directive';
import { ProjectMemberRole } from '../../generated/v1/project_pb';
import '@nldd/design-system/combo-box';

import '@nldd/design-system/button';
import '@nldd/design-system/cell';
import '@nldd/design-system/form';
import '@nldd/design-system/form-actions';
import '@nldd/design-system/form-field';
import '@nldd/design-system/inline-dialog';
import '@nldd/design-system/list';
import '@nldd/design-system/list-item';
import '@nldd/design-system/menu';
import '@nldd/design-system/page';
import '@nldd/design-system/radio-button';
import '@nldd/design-system/sheet';
import '@nldd/design-system/simple-section';
import '@nldd/design-system/spacer';
import '@nldd/design-system/spacer-cell';
import '@nldd/design-system/text-cell';
import '@nldd/design-system/title';
import '@nldd/design-system/top-title-bar';
import '@nldd/design-system/validation-list';

const stringToRole = (value: string): ProjectMemberRole =>
  value === 'admin' ? ProjectMemberRole.ADMIN : ProjectMemberRole.VIEWER;

/**
 * Adding someone to a project, from wherever you were. The shell owns this sheet
 * rather than the member list, so the toolbar can open it over any page; it
 * therefore loads the users it can offer itself.
 */
@Component({
  selector: 'app-add-project-member-sheet',
  imports: [ReactiveFormsModule, SheetSyncDirective, AutofocusDirective],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './add-project-member-sheet.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class AddProjectMemberSheetComponent {
  readonly projectId = input.required<string>();

  readonly show = input(false);

  @Output() closed = new EventEmitter<void>();

  private fb = inject(FormBuilder);

  private projectClient = inject(PROJECT);

  private idempotency = createIdempotencyRef();

  private notificationService = inject(NotificationService);

  private organizationData = inject(OrganizationDataService);

  /** Falls back to the neutral word while the organization data is still
   *  loading, so the sheet title never reads "Add member to undefined". */
  projectName = computed(() => {
    const found = this.organizationData.getProjectById(this.projectId())?.project;
    return found?.alias || found?.name || 'this project';
  });

  availableUsers = signal<{ id: string; name: string }[]>([]);

  isAddingMember = signal(false);

  memberForm = this.fb.group({
    userId: ['', Validators.required],
    permission: ['viewer', Validators.required],
  });

  private selectedUserId = signal('');

  selectedUserName = computed(() => {
    const id = this.selectedUserId();
    return this.availableUsers().find((user) => user.id === id)?.name ?? '';
  });

  /** Set by pressing Add member without a choice: the button stays enabled, so
   *  the field is what says what is missing. */
  private userFieldTouched = signal(false);

  userFieldInvalid = computed(() => this.userFieldTouched() && !this.selectedUserId());

  permissionOptions = [
    {
      value: 'viewer',
      label: 'Project viewer',
      description: 'Can see the project, its members and its namespaces.',
    },
    {
      value: 'admin',
      label: 'Project admin',
      description: 'Can also edit the project, manage its members and create namespaces.',
    },
  ];

  constructor() {
    // The sheet outlives the page it was opened over, so opening starts from
    // nothing and fetches what the form offers rather than trusting last time.
    effect(() => {
      if (!this.show()) return;
      this.selectedUserId.set('');
      this.userFieldTouched.set(false);
      this.memberForm.reset({ userId: '', permission: 'viewer' });
      this.load(this.projectId());
    });
  }

  private async load(projectId: string) {
    const [project, organizationMembers] = await Promise.all([
      firstValueFrom(this.projectClient.listProjectMembers({ projectId })).catch(() => null),
      // Through the shared cache, so the sidebar's count is of this same list.
      this.organizationData.reloadMembers().catch(() => null),
    ]);

    const inProject = new Set((project?.members ?? []).map((member) => member.userId));
    const available = (organizationMembers ?? [])
      .filter(
        (member) =>
          member.externalRef && member.status === 'accepted' && !inProject.has(member.userId),
      )
      .map((member) => ({ id: member.userId, name: member.name }));
    this.availableUsers.set(available);
  }

  onClose() {
    this.closed.emit();
  }

  /** The row is the target, the radio is the control. */
  selectPermission(value: string) {
    this.memberForm.get('permission')?.setValue(value);
  }

  onUserChange(event: Event) {
    const { value } = (event as CustomEvent<{ value: string }>).detail;
    this.selectedUserId.set(value);
    this.userFieldTouched.set(false);
    this.memberForm.get('userId')?.setValue(value);
    this.memberForm.get('userId')?.markAsDirty();
  }

  async saveMember(event?: Event) {
    // No defaultPrevented guard as in the other sheets: [formGroup] cancels the
    // submit before this runs, and the keydown that requested it is marked
    // handled too, so the guard would drop both. Both still arrive, though, so
    // the second one finds the first already out.
    event?.preventDefault();
    if (this.isAddingMember()) return;

    if (!this.selectedUserId()) {
      this.userFieldTouched.set(true);
      return;
    }

    this.isAddingMember.set(true);
    const role = stringToRole(this.memberForm.value.permission!);
    const userId = this.memberForm.value.userId!;

    try {
      await withIdempotency(
        (opts) =>
          this.projectClient.addProjectMember({ projectId: this.projectId(), userId, role }, opts),
        { signal: this.idempotency.reset() },
      );
      this.closed.emit();
      this.notificationService.success(
        `'${this.selectedUserName()}' added to ${this.projectName()}`,
      );
      // The member list is a page of its own, and it may well be the page behind
      // this sheet, so it hears about the new member from here.
      this.organizationData.membersChanged.update((count) => count + 1);
    } catch (err) {
      // The list's own retry banner belongs to that page and this sheet may be
      // standing over another one, so the failure reports where it can be seen.
      this.closed.emit();
      this.notificationService.error(
        err instanceof Error ? `Member not added: ${err.message}` : 'Member not added',
      );
    } finally {
      this.isAddingMember.set(false);
    }
  }
}
