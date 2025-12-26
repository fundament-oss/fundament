import { Component, ViewChild, ElementRef, AfterViewInit, inject } from '@angular/core';
import { CommonModule } from '@angular/common';
import { ReactiveFormsModule, FormBuilder, FormGroup, Validators } from '@angular/forms';
import { Title } from '@angular/platform-browser';
import { Router } from '@angular/router';
import { ProgressStepperComponent } from '../progress-stepper/progress-stepper.component';
import { ADD_CLUSTER_STEPS } from './add-cluster.constants';

@Component({
  selector: 'app-add-cluster',
  standalone: true,
  imports: [CommonModule, ReactiveFormsModule, ProgressStepperComponent],
  templateUrl: './add-cluster.component.html',
  styleUrl: './add-cluster.component.css',
})
export class AddClusterComponent implements AfterViewInit {
  @ViewChild('clusterNameInput') clusterNameInput!: ElementRef<HTMLInputElement>;

  private titleService = inject(Title);
  private router = inject(Router);
  private fb = inject(FormBuilder);

  // Progress stepper
  steps = ADD_CLUSTER_STEPS;
  currentStepIndex = 0;

  // Form
  clusterForm: FormGroup;

  // Dropdown options based on Gardener
  regions = [
    { value: 'nl1', label: 'NL1' },
    { value: 'nl2', label: 'NL2' },
    { value: 'nl3', label: 'NL3' },
  ];

  kubernetesVersions = ['1.34.x', '1.28.x', '1.27.x', '1.26.x', '1.25.x'];

  constructor() {
    this.titleService.setTitle('Add cluster components — Fundament Console');

    this.clusterForm = this.fb.group({
      clusterName: [
        '',
        [
          Validators.required,
          Validators.maxLength(253),
          Validators.pattern(/^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$/),
        ],
      ],
      region: ['nl1', Validators.required],
      kubernetesVersion: ['1.34.x', Validators.required],
    });
  }

  ngAfterViewInit() {
    // Focus the cluster name input after the view is initialized
    this.clusterNameInput.nativeElement.focus();
  }

  get clusterName() {
    return this.clusterForm.get('clusterName');
  }

  getClusterNameError(): string {
    if (this.clusterName?.hasError('required')) {
      return 'The cluster name is required.';
    }
    if (this.clusterName?.hasError('maxlength')) {
      return 'The cluster name must not exceed 253 characters.';
    }
    if (this.clusterName?.hasError('pattern')) {
      return `The cluster name must contain only lowercase alphanumeric characters, '-' or '.', and start and end with an alphanumeric character.`;
    }
    return '';
  }

  onSubmit() {
    if (this.clusterForm.invalid) {
      this.clusterForm.markAllAsTouched();
      this.scrollToFirstError();
      return;
    }

    const clusterData = this.clusterForm.value;
    console.log('Creating cluster with data:', clusterData);

    // For now, just navigate to the next step
    // In a real app, this would make an API call
    this.router.navigate(['/add-cluster-nodes']);
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

  onCancel() {
    this.router.navigate(['/']);
  }
}
