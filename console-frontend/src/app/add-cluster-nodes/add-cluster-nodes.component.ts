import {
  Component,
  inject,
  ViewChildren,
  QueryList,
  ElementRef,
  AfterViewInit,
} from '@angular/core';
import { CommonModule } from '@angular/common';
import {
  ReactiveFormsModule,
  FormBuilder,
  FormArray,
  FormGroup,
  Validators,
  AbstractControl,
  ValidationErrors,
} from '@angular/forms';
import { Title } from '@angular/platform-browser';
import { Router, RouterLink } from '@angular/router';
import { ProgressStepperComponent } from '../progress-stepper/progress-stepper.component';
import { ADD_CLUSTER_STEPS } from '../add-cluster/add-cluster.constants';

@Component({
  selector: 'app-add-cluster-nodes',
  standalone: true,
  imports: [CommonModule, ReactiveFormsModule, ProgressStepperComponent, RouterLink],
  templateUrl: './add-cluster-nodes.component.html',
  styleUrl: './add-cluster-nodes.component.css',
})
export class AddClusterNodesComponent implements AfterViewInit {
  @ViewChildren('nodePoolNameInput') nodePoolNameInputs!: QueryList<ElementRef<HTMLInputElement>>;
  private titleService = inject(Title);
  private router = inject(Router);
  private fb = inject(FormBuilder);

  // Progress stepper
  steps = ADD_CLUSTER_STEPS;
  currentStepIndex = 1;

  // Form
  nodePoolsForm: FormGroup;

  // Dropdown options based on Gardener
  machineTypes = [
    { value: 'n1-standard-1', label: 'n1-standard-1 (1 vCPU, 3.75 GB RAM)' },
    { value: 'n1-standard-2', label: 'n1-standard-2 (2 vCPU, 7.5 GB RAM)' },
    { value: 'n1-standard-4', label: 'n1-standard-4 (4 vCPU, 15 GB RAM)' },
    { value: 'n1-standard-8', label: 'n1-standard-8 (8 vCPU, 30 GB RAM)' },
    { value: 'n1-highmem-2', label: 'n1-highmem-2 (2 vCPU, 13 GB RAM)' },
    { value: 'n1-highmem-4', label: 'n1-highmem-4 (4 vCPU, 26 GB RAM)' },
  ];

  constructor() {
    this.titleService.setTitle('Add cluster nodes — Fundament Console');

    this.nodePoolsForm = this.fb.group({
      nodePools: this.fb.array([this.createNodePoolFormGroup()]),
    });
  }

  get nodePools(): FormArray {
    return this.nodePoolsForm.get('nodePools') as FormArray;
  }

  createNodePoolFormGroup(): FormGroup {
    return this.fb.group({
      name: [
        this.generateNodePoolName(),
        [
          Validators.required,
          Validators.maxLength(63),
          Validators.pattern(/^[a-z]([-a-z0-9]*[a-z0-9])?$/),
          this.uniqueNodePoolNameValidator.bind(this),
        ],
      ],
      machineType: ['n1-standard-1', Validators.required],
      autoscaleMin: [1, [Validators.required, Validators.min(1), Validators.max(100)]],
      autoscaleMax: [3, [Validators.required, Validators.min(1), Validators.max(100)]],
    });
  }

  private generateNodePoolName(): string {
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

  ngAfterViewInit() {
    this.nodePoolNameInputs.first?.nativeElement.focus();
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

  onSubmit() {
    if (this.nodePoolsForm.invalid) {
      this.nodePoolsForm.markAllAsTouched();
      this.scrollToFirstError();
      return;
    }

    const clusterData = this.nodePoolsForm.value;
    console.log('Creating cluster with data:', clusterData);

    // For now, just navigate to the next step
    // In a real app, this would make an API call
    this.router.navigate(['/add-cluster-plugins']);
  }

  private scrollToFirstError() {
    setTimeout(() => {
      const firstInvalidControl = document.querySelector('.ng-invalid:not(form)');
      if (firstInvalidControl) {
        firstInvalidControl.scrollIntoView({ behavior: 'smooth' });
        (firstInvalidControl as HTMLElement).focus();
      }
    }, 0);
  }
}
