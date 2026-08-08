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
} from '@angular/core';
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
  @Input() submitButtonText = 'Next step';

  @Input() set initialData(data: NodePoolData[] | null) {
    if (data && data.length > 0) {
      this.loadInitialData(data);
    }
  }

  // Region-scoped machine types from the catalog (names + display labels).
  // An empty list normalizes to null: which region you are in decides what a
  // machine type even means, so there is nothing sensible to offer before it.
  @Input() set machineTypeOptions(options: MachineTypeOption[] | null) {
    this.catalogOptions = options && options.length > 0 ? options : null;
    if (this.catalogOptions) {
      const opts = this.catalogOptions;
      // Re-anchor controls whose value is not an offered machine type. A pool
      // that was created in another region can carry one this region lacks.
      this.nodePools.controls.forEach((control) => {
        const mt = control.get('machineType');
        if (!opts.some((o) => o.value === mt?.value)) {
          mt?.setValue(opts[0].value);
        }
      });
      this.cdr.markForCheck();
    }
  }

  @Output() formSubmit = new EventEmitter<{ nodePools: NodePoolData[] }>();

  private fb = inject(FormBuilder);

  private cdr = inject(ChangeDetectorRef);

  // Form
  nodePoolsForm: FormGroup;

  private catalogOptions: MachineTypeOption[] | null = null;

  selectMachineType(index: number, value: string): void {
    const control = this.nodePools.at(index).get('machineType');
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
  }

  ngAfterViewInit(): void {
    this.cdr.markForCheck();
  }

  get nodePools(): FormArray {
    return this.nodePoolsForm.get('nodePools') as FormArray;
  }

  createNodePoolFormGroup(data?: NodePoolData): FormGroup {
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
      machineType: [this.initialMachineTypeValue(data), Validators.required],
      autoscaleMin: [
        data?.autoscaleMin || 1,
        [Validators.required, Validators.min(1), Validators.max(100)],
      ],
      autoscaleMax: [
        data?.autoscaleMax || 3,
        [Validators.required, Validators.min(1), Validators.max(100)],
      ],
    });
  }

  // The machineType control holds the machine type name. An existing pool keeps
  // its own type on sight: whether this region still offers it is a question for
  // the catalog, which answers in the machineTypeOptions setter.
  private initialMachineTypeValue(data?: NodePoolData): string {
    if (data?.machineType) return data.machineType;
    return this.machineTypes[0]?.value ?? '';
  }

  private loadInitialData(data: NodePoolData[]) {
    // Clear existing form array
    while (this.nodePools.length > 0) {
      this.nodePools.removeAt(0);
    }

    // Add all initial node pools
    data.forEach((pool) => {
      this.nodePools.push(this.createNodePoolFormGroup(pool));
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
