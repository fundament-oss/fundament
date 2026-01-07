import { Component, inject, computed, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { Router, RouterLink } from '@angular/router';
import { TitleService } from '../title.service';
import { InfoCircleIconComponent, ErrorIconComponent } from '../icons';
import { ClusterWizardStateService } from '../add-cluster-wizard-layout/cluster-wizard-state.service';
import { OrganizationApiService, CreateClusterRequest } from '../organization-api.service';

@Component({
  selector: 'app-add-cluster-summary',
  standalone: true,
  imports: [CommonModule, RouterLink, InfoCircleIconComponent, ErrorIconComponent],
  templateUrl: './add-cluster-summary.component.html',
})
export class AddClusterSummaryComponent {
  private titleService = inject(TitleService);
  private router = inject(Router);
  private organizationApi = inject(OrganizationApiService);
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

      console.log('Cluster created successfully:', response);

      // Reset wizard state and navigate to dashboard
      this.stateService.reset();
      this.router.navigate(['/']);
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
