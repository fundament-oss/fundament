import { Component, inject, ChangeDetectorRef } from '@angular/core';
import { CommonModule } from '@angular/common';
import { Router, RouterLink, RouterOutlet } from '@angular/router';
import { CheckmarkIconComponent } from '../icons';
interface ProgressStep {
  name: string;
  route: string;
}

@Component({
  selector: 'app-add-cluster-wizard-layout',
  standalone: true,
  imports: [CommonModule, RouterOutlet, RouterLink, CheckmarkIconComponent],
  templateUrl: './add-cluster-wizard-layout.component.html',
})
export class AddClusterWizardLayoutComponent {
  private router = inject(Router);
  private cdr = inject(ChangeDetectorRef);

  steps: ProgressStep[] = [
    { name: 'Basics', route: '/add-cluster' },
    { name: 'Worker nodes', route: '/add-cluster/nodes' },
    { name: 'Plugins', route: '/add-cluster/plugins' },
    { name: 'Summary', route: '/add-cluster/summary' },
  ];

  get currentStepIndex(): number {
    const currentRoute = this.router.url;
    // Find the last matching step (most specific route)
    // e.g., /add-cluster/nodes should match /add-cluster/nodes, not /add-cluster
    for (let i = this.steps.length - 1; i >= 0; i--) {
      if (currentRoute.startsWith(this.steps[i].route)) {
        return i;
      }
    }
    return -1;
  }

  onActivate() {
    this.cdr.markForCheck();
  }

  get currentStep() {
    return this.steps[this.currentStepIndex];
  }

  get isFirstStep(): boolean {
    return this.currentStepIndex === 0;
  }

  get isLastStep(): boolean {
    return this.currentStepIndex === this.steps.length - 1;
  }

  get previousRoute(): string | null {
    if (this.isFirstStep) return null;
    return this.steps[this.currentStepIndex - 1].route;
  }

  get nextRoute(): string | null {
    if (this.isLastStep) return null;
    return this.steps[this.currentStepIndex + 1].route;
  }

  onPrevious() {
    if (this.previousRoute) {
      this.router.navigate([this.previousRoute]);
    }
  }

  onNext() {
    if (this.nextRoute) {
      this.router.navigate([this.nextRoute]);
    }
  }

  onCancel() {
    this.router.navigate(['/']);
  }

  isCompleted(index: number): boolean {
    return index < this.currentStepIndex;
  }

  isActive(index: number): boolean {
    return index === this.currentStepIndex;
  }
}
