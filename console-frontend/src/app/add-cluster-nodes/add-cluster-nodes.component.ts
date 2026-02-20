import { Component, inject, ChangeDetectionStrategy } from '@angular/core';
import { CommonModule } from '@angular/common';
import { Router, RouterLink } from '@angular/router';
import { NgIcon, provideIcons } from '@ng-icons/core';
import { tablerArrowRight } from '@ng-icons/tabler-icons';
import { TitleService } from '../title.service';
import {
  SharedNodePoolsFormComponent,
  NodePoolData,
} from '../shared-node-pools-form/shared-node-pools-form.component';
import { ClusterWizardStateService } from '../add-cluster-wizard-layout/cluster-wizard-state.service';

@Component({
  selector: 'app-add-cluster-nodes',
  imports: [CommonModule, SharedNodePoolsFormComponent, RouterLink, NgIcon],
  viewProviders: [
    provideIcons({
      tablerArrowRight,
    }),
  ],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './add-cluster-nodes.component.html',
})
export default class AddClusterNodesComponent {
  private titleService = inject(TitleService);

  private router = inject(Router);

  private stateService = inject(ClusterWizardStateService);

  constructor() {
    this.titleService.setTitle('Add cluster nodes');
  }

  onFormSubmit(data: { nodePools: NodePoolData[] }) {
    // Save node pools to state
    this.stateService.updateNodePools(data.nodePools);
    this.stateService.markStepCompleted(1);

    // Navigate to the next step
    this.router.navigate(['/clusters/add/plugins']);
  }
}
