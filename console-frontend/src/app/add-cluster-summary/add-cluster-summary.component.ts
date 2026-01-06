import { Component, inject, computed } from '@angular/core';
import { CommonModule } from '@angular/common';
import { Router, RouterLink } from '@angular/router';
import { TitleService } from '../title.service';
import { InfoCircleIconComponent } from '../icons';
import { ClusterWizardStateService } from '../add-cluster-wizard-layout/cluster-wizard-state.service';

@Component({
  selector: 'app-add-cluster-summary',
  standalone: true,
  imports: [CommonModule, RouterLink, InfoCircleIconComponent],
  templateUrl: './add-cluster-summary.component.html',
})
export class AddClusterSummaryComponent {
  private titleService = inject(TitleService);
  private router = inject(Router);
  protected stateService = inject(ClusterWizardStateService);

  // Computed signal to access state
  protected state = computed(() => this.stateService.getState());

  constructor() {
    this.titleService.setTitle('Cluster summary');
  }

  onCreateCluster() {
    const wizardState = this.state();
    console.log('Creating cluster with summary:', wizardState);

    // For now, just navigate back to dashboard
    // In a real app, this would make an API call to create the cluster
    alert('Cluster is being created! (This is a demo)');
    this.router.navigate(['/']);
  }
}
