import { Component, inject } from '@angular/core';
import { CommonModule } from '@angular/common';
import { Title } from '@angular/platform-browser';
import { Router } from '@angular/router';
import {
  SharedNodePoolsFormComponent,
  NodePoolData,
} from '../shared-node-pools-form/shared-node-pools-form.component';

@Component({
  selector: 'app-cluster-nodes',
  standalone: true,
  imports: [CommonModule, SharedNodePoolsFormComponent],
  templateUrl: './cluster-nodes.component.html',
})
export class ClusterNodesComponent {
  private titleService = inject(Title);
  private router = inject(Router);

  constructor() {
    this.titleService.setTitle('Cluster nodes — Fundament Console');
  }

  onFormSubmit(data: { nodePools: NodePoolData[] }) {
    console.log('Saving cluster node changes:', data);

    // For now, just navigate back to cluster overview
    // In a real app, this would make an API call
    this.router.navigate(['/cluster-overview']);
  }

  onCancel() {
    this.router.navigate(['/cluster-overview']);
  }
}
