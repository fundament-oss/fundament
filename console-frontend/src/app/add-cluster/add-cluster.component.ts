import {
  Component,
  ViewChild,
  ElementRef,
  AfterViewInit,
  inject,
  OnInit,
  OnDestroy,
  ChangeDetectionStrategy,
} from '@angular/core';
import { ReactiveFormsModule, FormBuilder, FormGroup, Validators } from '@angular/forms';
import { Router } from '@angular/router';
import { Subscription } from 'rxjs';
import { TitleService } from '../title.service';
import { ClusterWizardStateService } from '../add-cluster-wizard-layout/cluster-wizard-state.service';

@Component({
  selector: 'app-add-cluster',
  imports: [ReactiveFormsModule],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './add-cluster.component.html',
})
export default class AddClusterComponent implements AfterViewInit, OnInit, OnDestroy {
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

  private nameSubscription?: Subscription;

  constructor() {
    this.titleService.setTitle('Add a cluster');

    this.clusterForm = this.fb.group({
      clusterName: ['', [Validators.required]],
      clusterSlug: [
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
    // Auto-populate slug from name
    this.nameSubscription = this.clusterForm.get('clusterName')!.valueChanges.subscribe(
      (name: string) => {
        const slug = this.toSlug(name);
        this.clusterForm.get('clusterSlug')!.setValue(slug);
      },
    );

    // Load existing state if available
    const state = this.stateService.getState();
    if (state.clusterName) {
      this.clusterForm.patchValue({
        clusterName: state.clusterName,
        clusterSlug: state.clusterSlug,
        region: state.region,
        kubernetesVersion: state.kubernetesVersion,
      });
    }
  }

  ngOnDestroy(): void {
    this.nameSubscription?.unsubscribe();
  }

  ngAfterViewInit() {
    // Focus the cluster name input after the view is initialized
    this.clusterNameInput.nativeElement.focus();
  }

  get clusterName() {
    return this.clusterForm.get('clusterName');
  }

  get clusterSlug() {
    return this.clusterForm.get('clusterSlug');
  }

  private toSlug(name: string): string {
    return name
      .normalize('NFD')
      .replace(/[\u0300-\u036f]/g, '') // Remove diacritics
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, '-') // Replace non-alphanumeric with dashes
      .substring(0, 253) // Truncate
      .replace(/^-+|-+$/g, ''); // Remove leading/trailing dashes
  }

  getClusterNameError(): string {
    if (this.clusterName?.hasError('required')) {
      return 'The cluster name is required.';
    }
    return '';
  }

  getClusterSlugError(): string {
    if (this.clusterSlug?.hasError('required')) {
      return 'The cluster slug is required.';
    }
    if (this.clusterSlug?.hasError('maxlength')) {
      return 'The cluster slug must not exceed 253 characters.';
    }
    if (this.clusterSlug?.hasError('pattern')) {
      return `The cluster slug must contain only lowercase alphanumeric characters, '-' or '.', and start and end with an alphanumeric character.`;
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
      clusterSlug: clusterData.clusterSlug,
      region: clusterData.region,
      kubernetesVersion: clusterData.kubernetesVersion,
    });
    this.stateService.markStepCompleted(0);

    // Navigate to the next step
    this.router.navigate(['/clusters/add/nodes']);
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
