import { Component, inject } from '@angular/core';
import { CommonModule } from '@angular/common';
import { Title } from '@angular/platform-browser';
import { Router, RouterLink } from '@angular/router';
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
  private titleService = inject(Title);
  private router = inject(Router);

  constructor() {
    this.titleService.setTitle('Add cluster nodes — Fundament Console');
  }

  onFormSubmit(data: { nodePools: NodePoolData[] }) {
    console.log('Creating cluster with data:', data);

    // For now, just navigate to the next step
    // In a real app, this would make an API call
    this.router.navigate(['/add-cluster/plugins']);
  }
}
