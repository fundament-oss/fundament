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
  // from the API. The form controls hold the catalog ids.
  private catalogRegions: Region[] = [];

  regions = signal<{ value: string; label: string }[]>([]);

  kubernetesVersions = signal<{ value: string; label: string }[]>([]);

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

    this.regions.set(this.catalogRegions.map((r) => ({ value: r.id, label: r.name })));

    // Restore existing wizard state, else default to the first region.
    const state = this.stateService.getState();
    const restoredRegion =
      state.regionId && this.catalogRegions.find((r) => r.id === state.regionId);
    const region = restoredRegion || this.catalogRegions[0];
    if (region) {
      this.clusterForm.get('region')?.setValue(region.id);
      this.refreshVersions(region, state.kubernetesVersionId);
    }
    if (state.clusterName) {
      this.clusterForm.patchValue({ clusterName: state.clusterName });
    }
  }

  // Recompute the version options for the selected region; keep the requested
  // version when the region offers it, else fall back to the first offered.
  private refreshVersions(region: Region, preferredVersionId?: string) {
    const versions = region.kubernetesVersions.map((v) => ({ value: v.id, label: v.version }));
    this.kubernetesVersions.set(versions);

    const preferred = preferredVersionId && versions.find((v) => v.value === preferredVersionId);
    this.clusterForm
      .get('kubernetesVersion')
      ?.setValue(preferred ? preferred.value : (versions[0]?.value ?? ''));
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
    const region = this.catalogRegions.find((r) => r.id === clusterData.region);
    const version = region?.kubernetesVersions.find((v) => v.id === clusterData.kubernetesVersion);
    if (!region || !version) {
      return;
    }

    // Save state: the catalog ids drive the create request, the names the display.
    this.stateService.updateBasicInfo({
      clusterName: clusterData.clusterName,
      region: region.name,
      regionId: region.id,
      kubernetesVersion: version.version,
      kubernetesVersionId: version.id,
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
      const region = this.catalogRegions.find((r) => r.id === value);
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
