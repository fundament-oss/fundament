import {
  Component,
  inject,
  computed,
  effect,
  viewChild,
  ElementRef,
  afterNextRender,
  Injector,
  Input,
  Output,
  EventEmitter,
  OnDestroy,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import type { StepIndicatorStatus } from '@nldd/design-system/step-indicator';
import { NewNewClusterFormStateService } from './new-cluster-form-state.service';
import SheetSyncDirective, { rewireFormFields } from '../sheet-sync.directive';
import NewClusterComponent from '../new-cluster/new-cluster.component';
import NewClusterNodesComponent from '../new-cluster-nodes/new-cluster-nodes.component';
import NewClusterSummaryComponent from '../new-cluster-summary/new-cluster-summary.component';

interface ProgressStep {
  name: string;
}

@Component({
  selector: 'app-new-cluster-form',
  imports: [
    SheetSyncDirective,
    NewClusterComponent,
    NewClusterNodesComponent,
    NewClusterSummaryComponent,
  ],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './new-cluster-form.component.html',
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
})
export default class NewClusterFormComponent implements OnDestroy {
  @Input() show = false;

  @Output() closed = new EventEmitter<void>();

  private sheetRef = viewChild<ElementRef<HTMLElement>>('sheet');

  private injector = inject(Injector);

  protected stateService = inject(NewNewClusterFormStateService);

  constructor() {
    // Every step arrives in a sheet that is already open, where the two things
    // below used to be handled by the router activating a step component.
    effect(() => {
      this.stateService.stepIndex();
      this.afterStepRendered();
    });
  }

  /** Dismissing the sheet takes the form away and reveals the page it was
   *  covering, whichever page that is. */
  onClose(): void {
    this.closed.emit();
  }

  // The design system ships Dutch defaults; the console is in US English.
  readonly stepIndicatorTranslations = {
    'components.step-indicator.accessible-label': 'Progress',
    'components.step-indicator.status-past-label': 'Completed',
    'components.step-indicator.status-current-label': 'Current step',
    'components.step-indicator.status-future-label': 'Not started',
    'components.step-indicator.compact-text': 'Step {current} of {total}',
  };

  steps: ProgressStep[] = [{ name: 'Basics' }, { name: 'Node pools' }, { name: 'Summary' }];

  currentStepIndex = this.stateService.stepIndex;

  // nldd-step-indicator is 1-based; it drives the collapsed "step x of y" line
  // and is the fallback for items that carry no status of their own.
  currentStepNumber = computed(() => this.currentStepIndex() + 1);

  /** Names the step the back button returns to; empty on step one, where the
   *  title bar shows the flow's own title instead. */
  previousStepName = computed(() => {
    const index = this.currentStepIndex();
    return index > 0 ? this.steps[index - 1].name : null;
  });

  /** Every step names itself, short where its own heading is a full sentence:
   *  the bar stands in for that heading once you scroll past it. */
  currentStepTitle = computed(() => this.steps[this.currentStepIndex()].name);

  // Set per item rather than left to `current`: the form can be stepped back
  // into, and a step that is already filled in stays ticked when it does.
  stepStatus(index: number): StepIndicatorStatus {
    if (index === this.currentStepIndex()) return 'current';
    return this.stateService.isStepCompleted(index) ? 'past' : 'future';
  }

  ngOnDestroy(): void {
    this.stateService.reset();
  }

  private afterStepRendered() {
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
        // changes. That happens while the step is still being rendered, so the
        // heading it points at does not exist yet and the bar silently falls back
        // to its static state. Re-set the attribute now that the step is there.
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

  // Steps render as a button rather than a link: the anchor would live inside
  // the element's shadow DOM, out of routerLink's reach.
  goToStep(index: number) {
    if (!this.canNavigate(index)) return;
    this.stateService.goToStep(index);
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
