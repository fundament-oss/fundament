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
import DropdownSyncDirective from '../dropdown-sync.directive';
import { MachineTypeOption } from '../region-catalog.service';

export interface NodePoolData {
  name: string;
  machineType: string;
  // Catalog reference (region_machine_types id). Set when the form runs in
  // catalog mode (machineTypeOptions provided); the create request sends it.
  regionMachineTypeId?: string;
  autoscaleMin: number;
  autoscaleMax: number;
}

@Component({
  selector: 'app-shared-node-pools-form',
  imports: [ReactiveFormsModule, AutofocusDirective, DropdownSyncDirective],
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

  // Region-scoped machine types from the catalog. When set, the form runs in
  // catalog mode: the machineType controls hold region_machine_types ids and
  // the emitted pools carry regionMachineTypeId. Without it (legacy cluster
  // whose region is not in the catalog) the pools carry only the free-text
  // machine type name.
  @Input() set machineTypeOptions(options: MachineTypeOption[] | null) {
    // An empty list means "no catalog options" - normalize to legacy mode so the
    // dropdown keeps its fallback and submits do not emit bogus catalog ids.
    this.catalogOptions = options && options.length > 0 ? options : null;
    if (options && options.length > 0) {
      // Re-anchor controls whose value is not a valid option (initial default,
      // or names loaded before the catalog arrived).
      this.nodePools.controls.forEach((control) => {
        const mt = control.get('machineType');
        const value = mt?.value as string;
        if (!options.some((o) => o.value === value)) {
          const byName = options.find((o) => o.name === value);
          mt?.setValue(byName ? byName.value : options[0].value);
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

  // Fallback for legacy clusters without a catalog region.
  private static readonly legacyMachineTypes: MachineTypeOption[] = [
    { value: 'n1-standard-1', label: 'n1-standard-1 (1 vCPU, 3.75 GB RAM)', name: 'n1-standard-1' },
    { value: 'n1-standard-2', label: 'n1-standard-2 (2 vCPU, 7.5 GB RAM)', name: 'n1-standard-2' },
    { value: 'n1-standard-4', label: 'n1-standard-4 (4 vCPU, 15 GB RAM)', name: 'n1-standard-4' },
    { value: 'n1-standard-8', label: 'n1-standard-8 (8 vCPU, 30 GB RAM)', name: 'n1-standard-8' },
    { value: 'n1-highmem-2', label: 'n1-highmem-2 (2 vCPU, 13 GB RAM)', name: 'n1-highmem-2' },
    { value: 'n1-highmem-4', label: 'n1-highmem-4 (4 vCPU, 26 GB RAM)', name: 'n1-highmem-4' },
  ];

  get machineTypes(): MachineTypeOption[] {
    return this.catalogOptions ?? SharedNodePoolsFormComponent.legacyMachineTypes;
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

  // The machineType control holds the option VALUE: the region_machine_types id
  // in catalog mode, the machine type name in legacy mode.
  private initialMachineTypeValue(data?: NodePoolData): string {
    const options = this.machineTypes;
    if (data) {
      if (data.regionMachineTypeId && options.some((o) => o.value === data.regionMachineTypeId)) {
        return data.regionMachineTypeId;
      }
      const byName = options.find((o) => o.name === data.machineType);
      if (byName) {
        return byName.value;
      }
    }
    return options[0]?.value ?? '';
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

    const raw = this.nodePoolsForm.value as { nodePools: NodePoolData[] };
    const catalogMode = this.catalogOptions !== null;
    const nodePools = raw.nodePools.map((pool) => {
      // The control value is the option value; resolve the display name from it.
      const option = this.machineTypes.find((o) => o.value === pool.machineType);
      return {
        name: pool.name,
        machineType: option?.name ?? pool.machineType,
        regionMachineTypeId: catalogMode ? pool.machineType : undefined,
        autoscaleMin: pool.autoscaleMin,
        autoscaleMax: pool.autoscaleMax,
      };
    });

    this.formSubmit.emit({ nodePools });
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
