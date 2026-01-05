import { Component, inject } from '@angular/core';
import { CommonModule } from '@angular/common';
import { Router, RouterLink } from '@angular/router';
import { TitleService } from '../title.service';
import {
  SharedNodePoolsFormComponent,
  NodePoolData,
} from '../shared-node-pools-form/shared-node-pools-form.component';
import { ArrowRightIconComponent } from '../icons';

@Component({
  selector: 'app-add-cluster-nodes',
  standalone: true,
  imports: [CommonModule, SharedNodePoolsFormComponent, RouterLink, ArrowRightIconComponent],
  templateUrl: './add-cluster-nodes.component.html',
})
export class AddClusterNodesComponent {
  private titleService = inject(TitleService);
  private router = inject(Router);

  get clusterId(): string | null {
    const currentRoute = this.router.url;
    const clusterIdMatch = currentRoute.match(/\/add-cluster\/([^/]+)/);
    if (
      clusterIdMatch &&
      clusterIdMatch[1] !== 'nodes' &&
      clusterIdMatch[1] !== 'plugins' &&
      clusterIdMatch[1] !== 'summary'
    ) {
      return clusterIdMatch[1];
    }
    return null;
  }

  constructor() {
    this.titleService.setTitle('Add cluster nodes');
  }

  onFormSubmit(data: { nodePools: NodePoolData[] }) {
    console.log('Creating cluster with data:', data);

    // For now, just navigate to the next step
    // In a real app, this would make an API call
    this.router.navigate(['/add-cluster/plugins']);
  }
}
