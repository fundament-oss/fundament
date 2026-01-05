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

  get steps(): ProgressStep[] {
    const cid = this.clusterId;
    if (cid) {
      return [
        { name: 'Basics', route: `/add-cluster/${cid}` },
        { name: 'Worker nodes', route: `/add-cluster/${cid}/nodes` },
        { name: 'Plugins', route: `/add-cluster/${cid}/plugins` },
        { name: 'Summary', route: `/add-cluster/${cid}/summary` },
      ];
    }
    return [
      { name: 'Basics', route: '/add-cluster' },
      { name: 'Worker nodes', route: '/add-cluster/nodes' },
      { name: 'Plugins', route: '/add-cluster/plugins' },
      { name: 'Summary', route: '/add-cluster/summary' },
    ];
  }

  get currentStepIndex(): number {
    const currentRoute = this.router.url;
    
    // Extract clusterId if present in the URL
    const clusterIdMatch = currentRoute.match(/\/add-cluster\/([^/]+)/);
    const hasClusterId = clusterIdMatch && clusterIdMatch[1] !== 'nodes' && clusterIdMatch[1] !== 'plugins' && clusterIdMatch[1] !== 'summary';
    
    // Check for exact matches first
    if (currentRoute === '/add-cluster' || (hasClusterId && currentRoute === `/add-cluster/${clusterIdMatch![1]}`)) {
      return 0; // Basics
    }
    if (currentRoute.includes('/nodes')) {
      return 1; // Worker nodes
    }
    if (currentRoute.includes('/plugins')) {
      return 2; // Plugins
    }
    if (currentRoute.includes('/summary')) {
      return 3; // Summary
    }
    
    return 0; // Default to Basics
  }
  
  get clusterId(): string | null {
    const currentRoute = this.router.url;
    const clusterIdMatch = currentRoute.match(/\/add-cluster\/([^/]+)/);
    if (clusterIdMatch && clusterIdMatch[1] !== 'nodes' && clusterIdMatch[1] !== 'plugins' && clusterIdMatch[1] !== 'summary') {
      return clusterIdMatch[1];
    }
    return null;
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
