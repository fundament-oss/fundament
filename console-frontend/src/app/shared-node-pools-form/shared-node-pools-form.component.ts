import {
  AfterViewInit,
  ChangeDetectorRef,
  Component,
  Input,
  Output,
  EventEmitter,
  inject,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
  signal,
} from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import {
  ReactiveFormsModule,
  FormBuilder,
  FormArray,
  FormGroup,
  Validators,
  AbstractControl,
  ValidationErrors,
} from '@angular/forms';
import AutofocusDirective from '../autofocus.directive';
import { MachineTypeOption } from '../region-catalog.service';
import { NodeQuotas } from '../node-quota.service';
import { PoolDraft, PoolRoom, poolRoom, roomForNewPool } from './node-pool-room';

import '@nldd/design-system/activity-indicator';
import '@nldd/design-system/box';
import '@nldd/design-system/button';
import '@nldd/design-system/cell';
import '@nldd/design-system/collection';
import '@nldd/design-system/container';
import '@nldd/design-system/form';
import '@nldd/design-system/form-actions';
import '@nldd/design-system/form-field';
import '@nldd/design-system/list';
import '@nldd/design-system/list-item';
import '@nldd/design-system/number-field';
import '@nldd/design-system/radio-button';
import '@nldd/design-system/spacer-cell';
import '@nldd/design-system/text-cell';
import '@nldd/design-system/text-field';
import '@nldd/design-system/title';
import '@nldd/design-system/validation-list';

/** A new pool's maximum, when its machine type has the room for it. */
const DEFAULT_AUTOSCALE_MAX = 3;

/** A new pool starts on the default maximum, or on what is left when that is
 *  less, so the form does not open on a refusal. */
const startingAutoscaleMax = (room: number): number =>
  room > 0 ? Math.min(DEFAULT_AUTOSCALE_MAX, room) : DEFAULT_AUTOSCALE_MAX;

export interface NodePoolData {
  name: string;
  machineType: string; // catalog machine type name - what the create request sends
  autoscaleMin: number;
  autoscaleMax: number;
}

@Component({
  selector: 'app-shared-node-pools-form',
  imports: [ReactiveFormsModule, AutofocusDirective],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './shared-node-pools-form.component.html',
})
export class SharedNodePoolsFormComponent implements AfterViewInit {
  /** Whether the form has been submitted at least once. Before that, a field
   * that is not finished yet is not a mistake, only unfinished. */
  formSubmitted = signal(false);

  @Input() submitButtonText = 'Next step';

  /**
   * The pools to start from, for a form that edits pools that already exist.
   * An empty list is an answer, not a missing one: a cluster with no pools has
   * to look like a cluster with no pools, or the sheet invents one — a pool
   * with a generated name that the page behind the sheet rightly says is not
   * there, and that saving would then create.
   *
   * Null is the absence of an answer, which is what the create wizard binds:
   * nothing exists yet there, so the form keeps the one pool it starts with.
   */
  @Input() set initialData(data: NodePoolData[] | null) {
    if (data) {
      this.loadPools(data, { existing: true });
    }
  }

  /**
   * Pools filled in earlier that do not exist yet, for the create wizard when
   * you come back to this step: the form shows them as you left them. Unlike
   * `initialData` they are new pools, so the quota counts them as growth and
   * not as nodes the organization already has in use.
   */
  @Input() set draftData(data: NodePoolData[] | null | undefined) {
    if (data && data.length > 0) {
      this.loadPools(data, { existing: false });
    }
  }

  // Region-scoped machine types from the catalog (names + display labels).
  // An empty list normalizes to null: which region you are in decides what a
  // machine type even means, so there is nothing sensible to offer before it.
  @Input() set machineTypeOptions(options: MachineTypeOption[] | null) {
    this.catalogOptions = options && options.length > 0 ? options : null;
    if (this.catalogOptions) {
      const opts = this.catalogOptions;
      // Re-anchor new pools whose value is not an offered machine type. A pool
      // that exists keeps its own, offered here or not: saving another type
      // replaces the pool, so a save that only changes some other pool would
      // swap this one out.
      this.nodePools.controls.forEach((control) => {
        const mt = control.get('machineType');
        if (!this.loadedPools.has(control) && !opts.some((o) => o.value === mt?.value)) {
          mt?.setValue(this.preferredMachineType());
        }
      });
      this.cdr.markForCheck();
    }
  }

  /**
   * The organization's node quotas in the cluster's region, or null while they
   * are loading or when they could not be read. Without them the form bounds
   * nothing and leaves it to the API, which is the authority either way: the
   * room can be used up between loading the form and saving it.
   */
  @Input() set nodeQuotas(quotas: NodeQuotas | null) {
    this.quotas = quotas;
    if (quotas) {
      // A new pool nobody picked a machine type or maximum for yet moves off a
      // type with no room left for it, and down to the room its type has left,
      // so the form does not open on a refusal.
      this.nodePools.controls.forEach((control, index) => {
        if (this.originals.has(control.get('name')?.value)) return;
        const mt = control.get('machineType');
        if (
          mt?.pristine &&
          !this.loadedPools.has(control) &&
          poolRoom(index, this.poolDrafts(), this.originals, quotas).limit <= 0
        ) {
          mt.setValue(this.preferredMachineType(), { emitEvent: false });
        }
        const max = control.get('autoscaleMax');
        if (max?.pristine) {
          const { limit } = poolRoom(index, this.poolDrafts(), this.originals, quotas);
          max.setValue(Math.min(Number(max.value), startingAutoscaleMax(limit)), {
            emitEvent: false,
          });
        }
      });
    }
    this.revalidateQuotas();
    this.cdr.markForCheck();
  }

  @Output() formSubmit = new EventEmitter<{ nodePools: NodePoolData[] }>();

  private fb = inject(FormBuilder);

  private cdr = inject(ChangeDetectorRef);

  // Form
  nodePoolsForm: FormGroup;

  private catalogOptions: MachineTypeOption[] | null = null;

  private quotas: NodeQuotas | null = null;

  /** The pools the form was loaded with, by name, as they are counted in the
   *  quota's "in use". Empty in the create wizard. */
  private originals: ReadonlyMap<string, PoolDraft> = new Map();

  /** The pool groups loaded from `initialData`, renamed or not. Their machine
   *  type is fixed, as the user docs say: saving another one would not convert
   *  the pool but replace it, rolling its nodes to the new type one by one. */
  private loadedPools = new WeakSet<AbstractControl>();

  /** Whether the pool at `index` exists already, so its machine type is fixed. */
  isExistingPool(index: number): boolean {
    return this.loadedPools.has(this.nodePools.at(index));
  }

  /** The catalog label for a machine type, or its bare name when the region
   *  does not offer it (a pool from before the catalog can carry one). */
  machineTypeLabel(value: string): string {
    return this.machineTypes.find((mt) => mt.value === value)?.label ?? value;
  }

  selectMachineType(index: number, value: string): void {
    if (this.isExistingPool(index)) return;
    const control = this.nodePools.at(index).get('machineType');
    control?.setValue(value);
    control?.markAsDirty();
  }

  setAutoscaleMax(index: number, value: number): void {
    const control = this.nodePools.at(index).get('autoscaleMax');
    control?.setValue(value);
    control?.markAsDirty();
  }

  get machineTypes(): MachineTypeOption[] {
    return this.catalogOptions ?? [];
  }

  /** Nothing renders before the catalog is in. There used to be a list of n1-*
   *  types to fall back on, and it did two kinds of damage: it flashed six
   *  options that this region does not offer, and an existing pool's own type
   *  was not among them, so the pool silently re-anchored to whatever came
   *  first and saving would have resized a running pool. */
  get isLoadingMachineTypes(): boolean {
    return this.catalogOptions === null;
  }

  constructor() {
    this.nodePoolsForm = this.fb.group({
      nodePools: this.fb.array([this.createNodePoolFormGroup()]),
    });
    // Pools of the same machine type share the room, so a change to one pool's
    // maximum or machine type can make another one fit, or stop fitting.
    this.nodePools.valueChanges.pipe(takeUntilDestroyed()).subscribe(() => this.revalidateQuotas());
  }

  ngAfterViewInit(): void {
    this.cdr.markForCheck();
  }

  get nodePools(): FormArray {
    return this.nodePoolsForm.get('nodePools') as FormArray;
  }

  createNodePoolFormGroup(data?: NodePoolData): FormGroup {
    const machineType = this.initialMachineTypeValue(data);
    return this.fb.group({
      name: [
        data?.name || SharedNodePoolsFormComponent.generateNodePoolName(),
        [
          Validators.required,
          Validators.maxLength(63),
          Validators.pattern(/^[a-z]([-a-z0-9]*[a-z0-9])?$/),
          this.uniqueNodePoolNameValidator.bind(this),
        ],
      ],
      machineType: [machineType, [Validators.required, this.machineTypeQuotaValidator.bind(this)]],
      autoscaleMin: [data?.autoscaleMin || 1, [Validators.required, Validators.min(1)]],
      autoscaleMax: [
        data?.autoscaleMax || this.initialAutoscaleMax(machineType),
        [Validators.required, Validators.min(1), this.autoscaleMaxQuotaValidator.bind(this)],
      ],
    });
  }

  // The machineType control holds the machine type name. An existing pool keeps
  // its own type on sight: whether this region still offers it is a question for
  // the catalog, which answers in the machineTypeOptions setter.
  private initialMachineTypeValue(data?: NodePoolData): string {
    if (data?.machineType) return data.machineType;
    return this.preferredMachineType();
  }

  /** For a new pool: the first offered machine type that still has room, so a
   *  form does not open on a type the organization has no quota for. */
  private preferredMachineType(): string {
    const first = this.machineTypes[0]?.value ?? '';
    const { quotas } = this;
    if (!quotas) return first;
    const pools = this.nodePoolsForm ? this.poolDrafts() : [];
    return (
      this.machineTypes.find((mt) => roomForNewPool(mt.value, pools, this.originals, quotas) > 0)
        ?.value ?? first
    );
  }

  /** For a new pool: the default maximum, or the room its machine type has
   *  left next to the pools already in the form when that is less. */
  private initialAutoscaleMax(machineType: string): number {
    const { quotas } = this;
    if (!quotas || !this.nodePoolsForm) return DEFAULT_AUTOSCALE_MAX;
    return startingAutoscaleMax(
      roomForNewPool(machineType, this.poolDrafts(), this.originals, quotas),
    );
  }

  // Read from each pool's own controls, not from the array's value: while a
  // control is being validated, its new value has not reached the array yet.
  private poolDrafts(): PoolDraft[] {
    return this.nodePools.controls.map((control) => control.getRawValue() as PoolDraft);
  }

  /** The quota room for a pool in the form, or null when the quotas are not in. */
  private roomFor(pool: AbstractControl | null | undefined): PoolRoom | null {
    if (!this.quotas || !pool || !this.nodePoolsForm) return null;
    const index = this.nodePools.controls.indexOf(pool);
    if (index < 0) return null;
    return poolRoom(index, this.poolDrafts(), this.originals, this.quotas);
  }

  private machineTypeQuotaValidator(control: AbstractControl): ValidationErrors | null {
    const room = this.roomFor(control.parent);
    return room && room.grows && room.quota === 0 ? { noQuota: true } : null;
  }

  private autoscaleMaxQuotaValidator(control: AbstractControl): ValidationErrors | null {
    const room = this.roomFor(control.parent);
    if (!room || !room.grows || room.quota === 0) return null;
    return Number(control.value) > room.limit ? { quota: true } : null;
  }

  private revalidateQuotas() {
    this.nodePools.controls.forEach((control) => {
      control.get('machineType')?.updateValueAndValidity({ emitEvent: false });
      control.get('autoscaleMax')?.updateValueAndValidity({ emitEvent: false });
    });
  }

  /** Whether the organization has no quota for a machine type, which the list
   *  says on the type itself. False while the quotas are not in. */
  hasNoQuota(machineType: string): boolean {
    return !!this.quotas && !this.quotas.get(machineType)?.quota;
  }

  /** What the list says on a machine type for the pool at `index`: that the
   *  organization has no quota for it, or that the quota has no room left for
   *  this pool. Null while the quotas are not in, or when the type fits. */
  machineTypeNote(index: number, machineType: string): string | null {
    const { quotas } = this;
    if (!quotas) return null;
    if (this.hasNoQuota(machineType)) return 'No quota in this organization';
    const pools = this.poolDrafts();
    // An existing pool can always stay what it is on its own machine type.
    const original = this.originals.get(pools[index]?.name);
    if (original?.machineType === machineType) return null;
    const others = pools.filter((_, i) => i !== index);
    return roomForNewPool(machineType, others, this.originals, quotas) <= 0
      ? 'No room left in this organization'
      : null;
  }

  /** Said at the machine type as soon as it is picked: with no quota, nothing
   *  else in the pool can make it fit. */
  getMachineTypeQuotaError(index: number): string {
    const control = this.nodePools.at(index).get('machineType');
    if (!control?.hasError('noQuota')) return '';
    if (this.isExistingPool(index)) {
      // The quota counts a pool by its name: renamed, it is a new pool.
      const name = this.nodePools.at(index).get('name')?.value;
      return this.originals.has(name)
        ? `The organization has no quota for ${control.value} nodes, so this pool can keep or lower its maximum but not raise it. Ask a platform operator to set a quota.`
        : `The organization has no quota for ${control.value} nodes, and a renamed pool counts as a new one. Keep its name, or ask a platform operator to set a quota.`;
    }
    return `The organization has no quota for ${control.value} nodes. Choose another machine type, or ask a platform operator to set a quota.`;
  }

  getAutoscaleMaxError(index: number): string {
    const control = this.nodePools.at(index).get('autoscaleMax');
    if (control?.hasError('required')) return 'The autoscale maximum is required.';
    if (control?.hasError('min')) return 'The autoscale maximum must be at least 1.';
    if (control?.hasError('quota')) {
      const pool = this.nodePools.at(index);
      const room = this.roomFor(pool);
      // An existing pool is checked only when it grows, so what it has now is
      // allowed even when the room is below it: say that, not "at most".
      const original = this.originals.get(pool.get('name')?.value);
      if (
        room &&
        original?.machineType === room.machineType &&
        original.autoscaleMax > room.limit
      ) {
        return `This pool can stay at ${original.autoscaleMax} or shrink, but not grow: there is no room left in the organization's ${room.machineType} quota.`;
      }
      return room && room.limit > 0
        ? `At most ${room.limit}, or the organization goes over its ${room.machineType} quota.`
        : `There is no room left in the organization's ${room?.machineType ?? ''} quota.`;
    }
    return '';
  }

  private loadPools(data: NodePoolData[], { existing }: { existing: boolean }) {
    // Clear existing form array
    while (this.nodePools.length > 0) {
      this.nodePools.removeAt(0);
    }

    this.originals = existing ? new Map(data.map((pool) => [pool.name, pool])) : new Map();

    data.forEach((pool) => {
      const group = this.createNodePoolFormGroup(pool);
      // A draft's values were chosen already: the quotas arriving later must
      // not move them the way they move a pool nobody touched yet.
      if (existing) {
        this.loadedPools.add(group);
      } else {
        Object.values(group.controls).forEach((control) => control.markAsDirty());
      }
      this.nodePools.push(group);
    });
  }

  private static generateNodePoolName(): string {
    const randomSuffix = Array.from({ length: 3 }, () =>
      String.fromCharCode(97 + Math.floor(Math.random() * 26)),
    ).join('');
    return `node-pool-${randomSuffix}`;
  }

  private uniqueNodePoolNameValidator(control: AbstractControl): ValidationErrors | null {
    if (!control.value || !this.nodePoolsForm) {
      return null;
    }

    const currentName = control.value.toLowerCase();
    const nodePools = this.nodePools?.controls || [];

    const hasDuplicate = nodePools.some(
      (pool) => pool !== control.parent && pool.get('name')?.value?.toLowerCase() === currentName,
    );

    return hasDuplicate ? { duplicate: true } : null;
  }

  getNodePoolNameError(index: number): string {
    const nameControl = this.nodePools.at(index).get('name');
    if (nameControl?.hasError('required')) {
      return 'The node pool name is required.';
    }
    if (nameControl?.hasError('maxlength')) {
      return 'The node pool name must not exceed 63 characters.';
    }
    if (nameControl?.hasError('pattern')) {
      return `The node pool name must contain only lowercase alphanumeric characters or '-', start with an alphabetic character, and end with an alphanumeric character.`;
    }
    if (nameControl?.hasError('duplicate')) {
      return 'This node pool name is already in use. Please choose a unique name.';
    }
    return '';
  }

  onNodePoolNameInput(index: number, event: Event) {
    const value = (event as CustomEvent<{ value: string }>).detail.value;
    this.nodePools.at(index).get('name')?.setValue(value);
    this.nodePools.at(index).get('name')?.markAsDirty();
  }

  addNodePool() {
    this.nodePools.push(this.createNodePoolFormGroup());
    this.revalidateNodePoolNames();
  }

  removeNodePool(index: number) {
    if (this.nodePools.length > 1) {
      this.nodePools.removeAt(index);
      this.revalidateNodePoolNames();
    }
  }

  private revalidateNodePoolNames() {
    this.nodePools.controls.forEach((control) => {
      control.get('name')?.updateValueAndValidity();
    });
  }

  submit() {
    this.onSubmit();
  }

  onSubmit(event?: Event) {
    event?.preventDefault();
    this.formSubmitted.set(true);

    if (this.nodePoolsForm.invalid) {
      this.nodePoolsForm.markAllAsTouched();
      SharedNodePoolsFormComponent.scrollToFirstError();
      return;
    }

    this.formSubmit.emit(this.nodePoolsForm.value);
  }

  private static scrollToFirstError() {
    setTimeout(() => {
      const firstInvalidControl = document.querySelector('.ng-invalid:not(form)');
      if (firstInvalidControl) {
        firstInvalidControl.scrollIntoView({ behavior: 'smooth' });
        (firstInvalidControl as HTMLElement).focus();
      }
    }, 0);
  }
}
