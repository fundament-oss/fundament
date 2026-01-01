import { Component, inject } from '@angular/core';
import { CommonModule } from '@angular/common';
import { Title } from '@angular/platform-browser';
import { Router, RouterLink } from '@angular/router';
import { ProgressStepperComponent } from '../progress-stepper/progress-stepper.component';
import { SharedPluginsFormComponent } from '../shared-plugins-form/shared-plugins-form.component';
import { ADD_CLUSTER_STEPS } from '../add-cluster/add-cluster.constants';
import { ArrowRightIconComponent } from '../icons';

@Component({
  selector: 'app-add-cluster-plugins',
  standalone: true,
  imports: [
    CommonModule,
    ProgressStepperComponent,
    SharedPluginsFormComponent,
    RouterLink,
    ArrowRightIconComponent,
  ],
  templateUrl: './add-cluster-plugins.component.html',
})
export class AddClusterPluginsComponent {
  private titleService = inject(Title);
  private router = inject(Router);

  // Progress stepper
  steps = ADD_CLUSTER_STEPS;
  currentStepIndex = 2;

  constructor() {
    this.titleService.setTitle('Add cluster plugins — Fundament Console');
  }

  onFormSubmit(data: { preset: string; plugins: string[] }) {
    console.log('Creating cluster with data:', data);

    this.router.navigate(['/add-cluster-summary']);
  }
}
