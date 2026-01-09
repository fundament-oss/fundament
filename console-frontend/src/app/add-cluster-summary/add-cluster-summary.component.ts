import { Component, inject, computed, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { Router, RouterLink } from '@angular/router';
import { TitleService } from '../title.service';
import { ToastService } from '../toast.service';
import { ErrorIconComponent } from '../icons';
import { ClusterWizardStateService } from '../add-cluster-wizard-layout/cluster-wizard-state.service';
import { OrganizationApiService, CreateClusterRequest } from '../organization-api.service';

@Component({
  selector: 'app-add-cluster-summary',
  standalone: true,
  imports: [CommonModule, RouterLink, ErrorIconComponent],
  templateUrl: './add-cluster-summary.component.html',
})
export class AddClusterSummaryComponent {
  private titleService = inject(TitleService);
  private router = inject(Router);
  private organizationApi = inject(OrganizationApiService);
  private toastService = inject(ToastService);
  protected stateService = inject(ClusterWizardStateService);

  // Computed signal to access state
  protected state = computed(() => this.stateService.getState());

  // Error state
  protected errorMessage = signal<string | null>(null);
  protected isCreating = signal<boolean>(false);

  constructor() {
    this.titleService.setTitle('Cluster summary');
  }

  async onCreateCluster() {
    const wizardState = this.state();

    // Validate required fields
    if (!wizardState.clusterName || !wizardState.region || !wizardState.kubernetesVersion) {
      this.errorMessage.set('Missing required cluster information');
      return;
    }

    // Clear previous errors and set loading state
    this.errorMessage.set(null);
    this.isCreating.set(true);

    try {
      // Build the request
      const request: CreateClusterRequest = {
        name: wizardState.clusterName,
        region: wizardState.region,
        kubernetesVersion: wizardState.kubernetesVersion,
        nodePools: wizardState.nodePools,
        pluginIds: wizardState.plugins,
        pluginPreset: wizardState.preset,
      };

      // Call the API
      const response = await this.organizationApi.createCluster(request);

      // Reset wizard state
      this.stateService.reset();

      // Set toast message
      this.toastService.info('Your cluster is being provisioned. This may take a few minutes.');

      // Navigate to the cluster detail page
      this.router.navigate(['/clusters', response.clusterId]);
    } catch (error) {
      console.error('Failed to create cluster:', error);
      this.errorMessage.set(
        error instanceof Error ? error.message : 'Failed to create cluster. Please try again.',
      );
    } finally {
      this.isCreating.set(false);
    }
  }
}
