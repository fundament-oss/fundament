import {
  Component,
  inject,
  computed,
  signal,
  viewChild,
  ElementRef,
  afterNextRender,
  Injector,
  OnDestroy,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { Router, RouterOutlet } from '@angular/router';
import type { StepIndicatorStatus } from '@nldd/design-system/step-indicator';
import { ClusterWizardStateService } from './cluster-wizard-state.service';
import SheetSyncDirective, { rewireFormFields } from '../sheet-sync.directive';

interface ProgressStep {
  name: string;
  route: string;
}

@Component({
  selector: 'app-add-cluster-wizard-layout',
  imports: [RouterOutlet, SheetSyncDirective],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './add-cluster-wizard-layout.component.html',
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
})
export default class AddClusterWizardLayoutComponent implements OnDestroy {
  private router = inject(Router);

  private sheetRef = viewChild<ElementRef<HTMLElement>>('sheet');

  private injector = inject(Injector);

  /** Dismissing the sheet leaves the wizard, which unmounts this route and
   *  reveals the cluster list the sheet was covering. */
  onClose(): void {
    if (this.router.url.startsWith('/clusters/add')) {
      this.router.navigate(['/clusters']);
    }
  }

  protected stateService = inject(ClusterWizardStateService);

  // The design system ships Dutch defaults; the console is in US English.
  readonly stepIndicatorTranslations = {
    'components.step-indicator.accessible-label': 'Progress',
    'components.step-indicator.status-past-label': 'Completed',
    'components.step-indicator.status-current-label': 'Current step',
    'components.step-indicator.status-future-label': 'Not started',
    'components.step-indicator.compact-text': 'Step {current} of {total}',
  };

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

  /** Names the step the back button returns to; empty on step one, where the
   *  title bar shows the flow's own title instead. */
  previousStepName = computed(() => {
    const index = this.currentStepIndex();
    return index > 0 ? this.steps[index - 1].name : null;
  });

  /** Every step names itself, short where its own heading is a full sentence:
   *  the bar stands in for that heading once you scroll past it. */
  currentStepTitle = computed(() => this.steps[this.currentStepIndex()].name);

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

    // The sheet is portaled to document.body, which disconnects every
    // nldd-form-field in it and kills the MutationObserver that watches its
    // input's `invalid` attribute — the error text would stay hidden forever.
    // Rewiring has to wait for the step's own fields to be in the DOM.
    afterNextRender(
      () => {
        const sheet = this.sheetRef()?.nativeElement;
        if (!sheet) return;

        rewireFormFields(sheet);

        // nldd-top-title-bar resolves `collapse-anchor` once, when the attribute
        // changes. That happens while the step is still being routed in, so the
        // heading it points at does not exist yet and the bar silently falls back
        // to its static state. Re-set the attribute now that the step has rendered.
        const bar = sheet.querySelector('nldd-top-title-bar');
        const anchor = bar?.getAttribute('collapse-anchor');
        if (bar && anchor) {
          bar.removeAttribute('collapse-anchor');
          (bar as HTMLElement & { updateComplete?: Promise<unknown> }).updateComplete?.then(() => {
            bar.setAttribute('collapse-anchor', anchor);
          });
        }
      },
      { injector: this.injector },
    );
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
    this.router.navigate(['/clusters']);
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
