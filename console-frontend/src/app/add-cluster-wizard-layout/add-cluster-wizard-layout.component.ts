import { Component, inject, computed, signal } from '@angular/core';
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

  steps: ProgressStep[] = [
    { name: 'Basics', route: '/add-cluster' },
    { name: 'Worker nodes', route: '/add-cluster/nodes' },
    { name: 'Plugins', route: '/add-cluster/plugins' },
    { name: 'Summary', route: '/add-cluster/summary' },
  ];

  // Signal to track route changes
  private routeSignal = signal(this.router.url);

  // Computed signal for current step index
  currentStepIndex = computed(() => {
    const currentRoute = this.routeSignal();
    // Find the last matching step (most specific route)
    // e.g., /add-cluster/nodes should match /add-cluster/nodes, not /add-cluster
    for (let i = this.steps.length - 1; i >= 0; i--) {
      if (currentRoute.startsWith(this.steps[i].route)) {
        return i;
      }
    }
    return -1;
  });

  onActivate() {
    // Update the route signal when a new route is activated
    this.routeSignal.set(this.router.url);
  }

  // Computed signals for derived state
  currentStep = computed(() => this.steps[this.currentStepIndex()]);
  
  isFirstStep = computed(() => this.currentStepIndex() === 0);
  
  isLastStep = computed(() => this.currentStepIndex() === this.steps.length - 1);
  
  previousRoute = computed(() => {
    if (this.isFirstStep()) return null;
    return this.steps[this.currentStepIndex() - 1].route;
  });
  
  nextRoute = computed(() => {
    if (this.isLastStep()) return null;
    return this.steps[this.currentStepIndex() + 1].route;
  });

  onPrevious() {
    const prev = this.previousRoute();
    if (prev) {
      this.router.navigate([prev]);
    }
  }

  onNext() {
    const next = this.nextRoute();
    if (next) {
      this.router.navigate([next]);
    }
  }

  onCancel() {
    this.router.navigate(['/']);
  }

  isCompleted(index: number): boolean {
    return index < this.currentStepIndex();
  }

  isActive(index: number): boolean {
    return index === this.currentStepIndex();
  }
}
