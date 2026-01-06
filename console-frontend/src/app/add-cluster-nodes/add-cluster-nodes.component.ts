import { Component, inject } from '@angular/core';
import { CommonModule } from '@angular/common';
import { Router, RouterLink } from '@angular/router';
import { TitleService } from '../title.service';
import {
  SharedNodePoolsFormComponent,
  NodePoolData,
} from '../shared-node-pools-form/shared-node-pools-form.component';
import { ArrowRightIconComponent } from '../icons';
import { ClusterWizardStateService } from '../add-cluster-wizard-layout/cluster-wizard-state.service';

@Component({
  selector: 'app-add-cluster-nodes',
  standalone: true,
  imports: [CommonModule, SharedNodePoolsFormComponent, RouterLink, ArrowRightIconComponent],
  templateUrl: './add-cluster-nodes.component.html',
})
export class AddClusterNodesComponent {
  private titleService = inject(TitleService);
  private router = inject(Router);
  private stateService = inject(ClusterWizardStateService);

  constructor() {
    this.titleService.setTitle('Add cluster nodes');
  }

  onFormSubmit(data: { nodePools: NodePoolData[] }) {
    console.log('Creating cluster with data:', data);

    // Save node pools to state
    this.stateService.updateNodePools(data.nodePools);
    this.stateService.markStepCompleted(1);

    // Navigate to the next step
    this.router.navigate(['/add-cluster/plugins']);
  }
}
