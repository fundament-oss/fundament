import {
  Component,
  inject,
  computed,
  signal,
  OnDestroy,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { Router, RouterOutlet } from '@angular/router';
import type { StepIndicatorStatus } from '@nldd/design-system/step-indicator';
import { ClusterWizardStateService } from './cluster-wizard-state.service';

interface ProgressStep {
  name: string;
  route: string;
}

@Component({
  selector: 'app-add-cluster-wizard-layout',
  imports: [RouterOutlet],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './add-cluster-wizard-layout.component.html',
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
})
export default class AddClusterWizardLayoutComponent implements OnDestroy {
  private router = inject(Router);

  protected stateService = inject(ClusterWizardStateService);

  steps: ProgressStep[] = [
    { name: 'Basics', route: '/clusters/add' },
    { name: 'Node pools', route: '/clusters/add/nodes' },
    { name: 'Summary', route: '/clusters/add/summary' },
  ];

  // Signal to track route changes
  private routeSignal = signal(this.router.url);

  // Computed signal for current step index
  currentStepIndex = computed(() => {
    const currentRoute = this.routeSignal();
    // Find the last matching step (most specific route)
    // e.g., /clusters/add/nodes should match /clusters/add/nodes, not /clusters/add
    for (let i = this.steps.length - 1; i >= 0; i -= 1) {
      if (currentRoute.startsWith(this.steps[i].route)) {
        return i;
      }
    }
    return -1;
  });

  // nldd-step-indicator is 1-based; it drives the collapsed "step x of y" line
  // and is the fallback for items that carry no status of their own.
  currentStepNumber = computed(() => Math.max(this.currentStepIndex() + 1, 1));

  // Set per item rather than left to `current`: the wizard can be stepped back
  // into, and a step that is already filled in stays ticked when it does.
  stepStatus(index: number): StepIndicatorStatus {
    if (index === this.currentStepIndex()) return 'current';
    return this.stateService.isStepCompleted(index) ? 'past' : 'future';
  }

  ngOnDestroy(): void {
    // Reset state when leaving the wizard
    this.stateService.reset();
  }

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

  // Steps render as a button rather than a link: the anchor would live inside
  // the element's shadow DOM, out of routerLink's reach.
  goToStep(index: number) {
    if (!this.canNavigate(index)) return;
    this.router.navigate([this.steps[index].route]);
  }

  canNavigate(index: number): boolean {
    // First step is always accessible
    if (index === 0) {
      return true;
    }
    // Other steps require first step to be completed
    return this.stateService.isFirstStepCompleted();
  }
}
