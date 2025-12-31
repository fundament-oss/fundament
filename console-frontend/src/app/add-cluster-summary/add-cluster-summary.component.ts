import { Component, inject } from '@angular/core';
import { CommonModule } from '@angular/common';
import { Title } from '@angular/platform-browser';
import { Router, RouterLink } from '@angular/router';
import { ProgressStepperComponent } from '../progress-stepper/progress-stepper.component';
import { ADD_CLUSTER_STEPS } from '../add-cluster/add-cluster.constants';

@Component({
  selector: 'app-add-cluster-summary',
  standalone: true,
  imports: [CommonModule, ProgressStepperComponent, RouterLink],
  templateUrl: './add-cluster-summary.component.html',
})
export class AddClusterSummaryComponent {
  private titleService = inject(Title);
  private router = inject(Router);

  // Progress stepper
  steps = ADD_CLUSTER_STEPS;
  currentStepIndex = 4;

  // Hardcoded summary data
  clusterSummary = {
    basics: {
      name: 'my-production-cluster',
      region: 'NL1',
      kubernetesVersion: '1.34.x',
    },
    nodePools: [
      {
        name: 'pool-xyz',
        machineType: 'n1-standard-1',
        autoscaleMin: 1,
        autoscaleMax: 3,
      },
    ],
    plugins: {
      preset: 'Haven+ preset',
      description: 'Includes monitoring, logging, security scanning, and backup solutions',
    },
    projects: [
      {
        name: 'my-project-1',
        namespaces: ['default'],
      },
      {
        name: 'my-project-2',
        namespaces: ['abc', 'xyz'],
      },
    ],
  };

  constructor() {
    this.titleService.setTitle('Cluster summary — Fundament Console');
  }

  onCreateCluster() {
    console.log('Creating cluster with summary:', this.clusterSummary);

    // For now, just navigate back to dashboard
    // In a real app, this would make an API call to create the cluster
    alert('Cluster is being created! (This is a demo)');
    this.router.navigate(['/']);
  }
}
