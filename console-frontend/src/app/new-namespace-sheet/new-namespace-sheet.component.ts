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
import { ReactiveFormsModule, FormBuilder, Validators, type ValidatorFn } from '@angular/forms';
import { create } from '@bufbuild/protobuf';
import { createIdempotencyRef, withIdempotency } from '../../connect/idempotency';
import { NotificationService } from '../notification.service';
import { OrganizationDataService } from '../organization-data.service';
import { CreateNamespaceRequestSchema } from '../../generated/v1/namespace_pb';
import SheetSyncDirective from '../sheet-sync.directive';
import AutofocusDirective from '../autofocus.directive';
import { NAMESPACE } from '../../connect/tokens';

import '@nldd/design-system/banner';
import '@nldd/design-system/button';
import '@nldd/design-system/form';
import '@nldd/design-system/form-actions';
import '@nldd/design-system/form-field';
import '@nldd/design-system/page';
import '@nldd/design-system/sheet';
import '@nldd/design-system/simple-section';
import '@nldd/design-system/spacer';
import '@nldd/design-system/text-field';
import '@nldd/design-system/title';
import '@nldd/design-system/top-title-bar';
import '@nldd/design-system/validation-list';

/** What a 63-character label leaves for project and namespace name together
 *  after "tnt-" and "--" (kubename.maxCombinedLength in Go). */
const MAX_COMBINED_NAME_LENGTH = 57;

/**
 * Making a namespace, from wherever you were. The shell owns this sheet rather
 * than the namespace list, so the toolbar can open it over any page.
 */
@Component({
  selector: 'app-new-namespace-sheet',
  imports: [ReactiveFormsModule, SheetSyncDirective, AutofocusDirective],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './new-namespace-sheet.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class NewNamespaceSheetComponent {
  readonly projectId = input.required<string>();

  readonly show = input(false);

  @Output() closed = new EventEmitter<void>();

  private fb = inject(FormBuilder);

  private namespaceClient = inject(NAMESPACE);

  private idempotency = createIdempotencyRef();

  private notificationService = inject(NotificationService);

  private organizationDataService = inject(OrganizationDataService);

  /** Named in the sheet title, so it says which project the namespace lands in. */
  projectName = computed(() => {
    const gevonden = this.organizationDataService.getProjectById(this.projectId())?.project;
    return gevonden?.alias || gevonden?.name || 'this project';
  });

  /** The cluster-side name is "tnt-<project name>--<namespace name>", a
   *  63-character DNS-1123 label, so the project name eats into the namespace's
   *  length. */
  technicalProjectName = computed(
    () => this.organizationDataService.getProjectById(this.projectId())?.project?.name ?? '',
  );

  maxNameLength = computed(() => {
    const technicalName = this.technicalProjectName();
    // Until the project is known, the most any one-character project allows.
    return MAX_COMBINED_NAME_LENGTH - (technicalName.length || 1);
  });

  /** The limit differs per project, so it says where the number comes from. */
  maxNameLengthText = computed(() => {
    const limit = `At most ${this.maxNameLength()} characters`;
    return this.technicalProjectName()
      ? `${limit} (project name + namespace name may be at most ${MAX_COMBINED_NAME_LENGTH})`
      : limit;
  });

  createErrorMessage = signal<string | null>(null);

  isCreatingNamespace = signal<boolean>(false);

  /**
   * Set by pressing Create. A field you have not filled in yet is not wrong, it
   * is unfinished, so nothing goes red until you say you are done. The rules
   * under the field are visible from the start as the requirements; they only
   * turn critical once this is true.
   */
  createAttempted = signal<boolean>(false);

  namespaceForm = this.fb.group({
    name: [
      '',
      [
        Validators.required,
        Validators.minLength(1),
        Validators.pattern(/^[a-z]([-a-z0-9]*[a-z0-9])?$/),
      ],
    ],
  });

  private nameLengthValidator: ValidatorFn | null = null;

  constructor() {
    // The length limit follows the project, which is known only once the input
    // is set and may load after the sheet opened.
    effect(() => {
      const control = this.namespaceForm.controls.name;
      if (this.nameLengthValidator) control.removeValidators(this.nameLengthValidator);
      this.nameLengthValidator = Validators.maxLength(this.maxNameLength());
      control.addValidators(this.nameLengthValidator);
      control.updateValueAndValidity();
    });

    // Opening is the moment to start from nothing: the sheet outlives the page
    // it was opened over, so the draft cannot be left over from last time.
    effect(() => {
      if (!this.show()) return;
      this.namespaceForm.reset();
      this.createErrorMessage.set(null);
    });
  }

  onClose() {
    this.closed.emit();
  }

  async createNamespace(event?: Event) {
    // A control that acts on Enter itself marks the event handled. Submitting
    // on that Enter would put the form in error while the user is still filling
    // it in.
    if (event?.defaultPrevented) return;

    event?.preventDefault();
    this.createErrorMessage.set(null);
    this.createAttempted.set(true);

    if (this.namespaceForm.invalid) {
      this.namespaceForm.markAllAsTouched();
      return;
    }

    try {
      this.isCreatingNamespace.set(true);

      const request = create(CreateNamespaceRequestSchema, {
        projectId: this.projectId(),
        name: this.namespaceForm.value.name!,
      });

      await withIdempotency((opts) => this.namespaceClient.createNamespace(request, opts), {
        signal: this.idempotency.reset(),
      });

      const name = this.namespaceForm.value.name!;

      this.closed.emit();
      this.notificationService.success(`Namespace '${name}' created`);

      // The list that shows namespaces is a page of its own, and it may well be
      // the page behind this sheet, so it hears about the new one from here.
      this.organizationDataService.namespacesChanged.update((count) => count + 1);
      // The projects, not the organization: the count that changed hangs off the
      // project, and reloading the organization drops the projects for good.
      // Swallowed because the namespace exists either way, and a failed refresh
      // reported as "Failed to create namespace" would say it does not.
      await this.organizationDataService.reloadProjectsAndNamespaces().catch(() => {});
    } catch (error) {
      this.createErrorMessage.set(
        error instanceof Error
          ? `Failed to create namespace: ${error.message}`
          : 'Failed to create namespace',
      );
    } finally {
      this.isCreatingNamespace.set(false);
    }
  }

  onNameInput(event: Event) {
    const value = (event as CustomEvent<{ value: string }>).detail.value;
    this.namespaceForm.get('name')?.setValue(value);
    this.namespaceForm.get('name')?.markAsDirty();
  }

  /**
   * Only after pressing Create. Typing towards a name that is not finished yet
   * is not a mistake, and a field that turns red on the second character reads
   * as one. The rules stand under the field the whole time, so there is
   * something to go on before the verdict.
   */
  isNameInvalid(): boolean {
    return this.createAttempted() && !!this.namespaceForm.get('name')?.invalid;
  }

  getNameError(): string {
    const nameControl = this.namespaceForm.get('name');
    if (nameControl?.hasError('required')) {
      return 'Namespace name is required.';
    }
    if (nameControl?.hasError('maxlength')) {
      return `${this.maxNameLengthText()}.`;
    }
    if (nameControl?.hasError('pattern')) {
      return 'Namespace name must start with a lowercase letter, end with a letter or number, and contain only lowercase letters, numbers, and hyphens.';
    }
    return '';
  }
}
