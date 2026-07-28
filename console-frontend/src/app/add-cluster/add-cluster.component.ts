import {
  Component,
  inject,
  OnInit,
  ChangeDetectionStrategy,
  signal,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { ReactiveFormsModule, FormBuilder, FormGroup, Validators } from '@angular/forms';
import { Router } from '@angular/router';
import { TitleService } from '../title.service';
import AutofocusDirective from '../autofocus.directive';
import { ClusterWizardStateService } from '../add-cluster-wizard-layout/cluster-wizard-state.service';
import { OrganizationDataService } from '../organization-data.service';
import { RegionCatalogService } from '../region-catalog.service';
import { Region } from '../../generated/v1/cluster_pb';

@Component({
  selector: 'app-add-cluster',
  imports: [ReactiveFormsModule, AutofocusDirective],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './add-cluster.component.html',
})
export default class AddClusterComponent implements OnInit {
  private titleService = inject(TitleService);

  private router = inject(Router);

  private fb = inject(FormBuilder);

  private stateService = inject(ClusterWizardStateService);

  private orgDataService = inject(OrganizationDataService);

  private regionCatalog = inject(RegionCatalogService);

  formSubmitted = signal(false);

  clusterNameExists = signal(false);

  // Form
  clusterForm: FormGroup;

  // Region catalog (regions with their offered kubernetes versions), loaded
  // from the API. Text-only: the form controls hold the catalog names.
  private catalogRegions: Region[] = [];

  regions = signal<{ value: string; label: string }[]>([]);

  kubernetesVersions = signal<string[]>([]);

  catalogError = signal<string | null>(null);

  constructor() {
    this.titleService.setTitle('Add a cluster');

    this.clusterForm = this.fb.group({
      clusterName: [
        '',
        [
          Validators.required,
          Validators.maxLength(253),
          Validators.pattern(/^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$/),
        ],
      ],
      region: ['', Validators.required],
      kubernetesVersion: ['', Validators.required],
    });
  }

  async ngOnInit(): Promise<void> {
    try {
      this.catalogRegions = await this.regionCatalog.getRegions();
    } catch {
      this.catalogError.set('Failed to load the available regions. Please try again later.');
      return;
    }

    if (this.catalogRegions.length === 0) {
      // Valid state until an operator seeds the catalog - explain instead of
      // presenting an empty, unsubmittable form.
      this.catalogError.set(
        'No regions are available yet. Ask an operator to seed the region catalog.',
      );
      return;
    }

    this.regions.set(this.catalogRegions.map((r) => ({ value: r.name, label: r.name })));

    // Restore existing wizard state, else default to the first region.
    const state = this.stateService.getState();
    const restoredRegion = state.region && this.catalogRegions.find((r) => r.name === state.region);
    const region = restoredRegion || this.catalogRegions[0];
    this.clusterForm.get('region')?.setValue(region.name);
    this.refreshVersions(region, state.kubernetesVersion);
    if (state.clusterName) {
      this.clusterForm.patchValue({ clusterName: state.clusterName });
    }
  }

  // Recompute the version options for the selected region; keep the requested
  // version when the region offers it, else fall back to the newest offered.
  private refreshVersions(region: Region, preferredVersion?: string) {
    this.kubernetesVersions.set(region.kubernetesVersions);

    const preferred = preferredVersion && region.kubernetesVersions.includes(preferredVersion);
    this.clusterForm
      .get('kubernetesVersion')
      ?.setValue(
        preferred ? preferredVersion : AddClusterComponent.newestVersion(region.kubernetesVersions),
      );
  }

  private static newestVersion(versions: string[]): string {
    return versions.reduce(
      (newest, v) => (AddClusterComponent.compareVersions(v, newest) > 0 ? v : newest),
      versions[0] ?? '',
    );
  }

  // The catalog orders versions as text, which ranks '1.9.0' above '1.34.0',
  // so compare the dot-separated components numerically instead.
  private static compareVersions(a: string, b: string): number {
    const partsA = a.split('.');
    const partsB = b.split('.');
    for (let i = 0; i < Math.max(partsA.length, partsB.length); i += 1) {
      const numA = parseInt(partsA[i], 10) || 0;
      const numB = parseInt(partsB[i], 10) || 0;
      if (numA !== numB) {
        return numA - numB;
      }
    }
    return 0;
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

  onClusterNameBlur() {
    const name = this.clusterName?.value as string;
    if (!name) {
      return;
    }
    const exists = this.orgDataService.clusterSummaries().some((c) => c.name === name);
    this.clusterNameExists.set(exists);
  }

  onClusterNameInput(event: Event) {
    const value = (event as CustomEvent<{ value: string }>).detail.value;
    this.clusterForm.get('clusterName')?.setValue(value);
    this.clusterForm.get('clusterName')?.markAsDirty();
    this.clusterNameExists.set(false);
  }

  onSubmit(event?: Event) {
    event?.preventDefault();

    this.formSubmitted.set(true);
    this.onClusterNameBlur();
    if (this.clusterForm.invalid || this.clusterNameExists()) {
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

  onRadioChange(controlName: string, event: Event) {
    const value = (event as CustomEvent<{ value: string }>).detail.value;
    this.clusterForm.get(controlName)?.setValue(value);

    // Region drives the offered kubernetes versions.
    if (controlName === 'region') {
      const region = this.catalogRegions.find((r) => r.name === value);
      if (region) {
        const currentVersion = this.clusterForm.get('kubernetesVersion')?.value as string;
        this.refreshVersions(region, currentVersion);
      }
    }
  }

  onCancel() {
    this.router.navigate(['/']);
  }
}
