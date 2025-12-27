import { Component, inject } from '@angular/core';
import { CommonModule } from '@angular/common';
import { Title } from '@angular/platform-browser';
import { Router, RouterLink } from '@angular/router';
import { ProgressStepperComponent } from '../progress-stepper/progress-stepper.component';
import {
  SharedNodePoolsFormComponent,
  NodePoolData,
} from '../shared-node-pools-form/shared-node-pools-form.component';
import { ADD_CLUSTER_STEPS } from '../add-cluster/add-cluster.constants';

@Component({
  selector: 'app-add-cluster-nodes',
  standalone: true,
  imports: [CommonModule, ProgressStepperComponent, SharedNodePoolsFormComponent, RouterLink],
  templateUrl: './add-cluster-nodes.component.html',
  styleUrl: './add-cluster-nodes.component.css',
})
export class AddClusterNodesComponent {
  private titleService = inject(Title);
  private router = inject(Router);

  // Progress stepper
  steps = ADD_CLUSTER_STEPS;
  currentStepIndex = 1;

  constructor() {
    this.titleService.setTitle('Add cluster nodes — Fundament Console');
  }

  onFormSubmit(data: { nodePools: NodePoolData[] }) {
    console.log('Creating cluster with data:', data);

    // For now, just navigate to the next step
    // In a real app, this would make an API call
    this.router.navigate(['/add-cluster-plugins']);
  }
}
