import {
  Component,
  ViewChild,
  ElementRef,
  AfterViewInit,
  inject,
  OnInit,
  ChangeDetectionStrategy,
} from '@angular/core';
import { CommonModule } from '@angular/common';
import { ReactiveFormsModule, FormBuilder, FormGroup, Validators } from '@angular/forms';
import { Router } from '@angular/router';
import { TitleService } from '../title.service';
import { ClusterWizardStateService } from '../add-cluster-wizard-layout/cluster-wizard-state.service';

@Component({
  selector: 'app-add-cluster',
  imports: [CommonModule, ReactiveFormsModule],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './add-cluster.component.html',
})
export default class AddClusterComponent implements AfterViewInit, OnInit {
  @ViewChild('clusterNameInput') clusterNameInput!: ElementRef<HTMLInputElement>;

  private titleService = inject(TitleService);

  private router = inject(Router);

  private fb = inject(FormBuilder);

  private stateService = inject(ClusterWizardStateService);

  // Form
  clusterForm: FormGroup;

  // Dropdown options based on Gardener
  // TODO: Fetch from API based on cloud profile
  regions = [{ value: 'local', label: 'Local' }];

  kubernetesVersions = ['1.31.1', '1.32.0', '1.33.0', '1.34.0'];

  constructor() {
    this.titleService.setTitle('Add cluster components');

    this.clusterForm = this.fb.group({
      clusterName: [
        '',
        [
          Validators.required,
          Validators.maxLength(253),
          Validators.pattern(/^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$/),
        ],
      ],
      region: ['local', Validators.required],
      kubernetesVersion: ['1.31.1', Validators.required],
    });
  }

  ngOnInit(): void {
    // Load existing state if available
    const state = this.stateService.getState();
    if (state.clusterName) {
      this.clusterForm.patchValue({
        clusterName: state.clusterName,
        region: state.region,
        kubernetesVersion: state.kubernetesVersion,
      });
    }
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
      AddClusterComponent.scrollToFirstError();
      return;
    }

    const clusterData = this.clusterForm.value;

    // Save state
    this.stateService.updateBasicInfo({
      clusterName: clusterData.clusterName,
      region: clusterData.region,
      kubernetesVersion: clusterData.kubernetesVersion,
    });
    this.stateService.markStepCompleted(0);

    // Navigate to the next step
    this.router.navigate(['/add-cluster/nodes']);
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

  onCancel() {
    this.router.navigate(['/']);
  }
}
