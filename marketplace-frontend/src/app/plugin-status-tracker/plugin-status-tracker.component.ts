import { Component, input, computed, ChangeDetectionStrategy } from '@angular/core';
import { CheckmarkIconComponent } from '../icons';
import { type SubmissionStatus } from '../status/submission-status';

type StepState = 'complete' | 'active' | 'error' | 'upcoming';

interface TrackerStep {
  name: string;
  state: StepState;
}

// Read-only status indicator for the plugin publishing pipeline:
// Pushed via functl -> Central review -> Publish. Non-navigable.
@Component({
  selector: 'app-plugin-status-tracker',
  imports: [CheckmarkIconComponent],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './plugin-status-tracker.component.html',
})
export default class PluginStatusTrackerComponent {
  status = input.required<SubmissionStatus>();

  steps = computed<TrackerStep[]>(() => {
    const reviewState = PluginStatusTrackerComponent.reviewState(this.status());
    const publishState: StepState = this.status() === 'approved' ? 'complete' : 'upcoming';

    return [
      { name: 'Pushed via functl', state: 'complete' },
      { name: 'Central review', state: reviewState },
      { name: 'Publish', state: publishState },
    ];
  });

  private static reviewState(status: SubmissionStatus): StepState {
    switch (status) {
      case 'approved':
        return 'complete';
      case 'changes_requested':
      case 'rejected':
        return 'error';
      case 'pending':
        return 'active';
      // A draft has not been submitted yet, and a withdrawn version has been
      // pulled back out of review, so both sit before the review step.
      case 'draft':
      case 'withdrawn':
        return 'upcoming';
      default:
        throw new Error(`unhandled status: ${status satisfies never}`);
    }
  }

  // Short hint shown under the tracker when the plugin needs author action.
  hint = computed<string | null>(() => {
    const status = this.status();
    switch (status) {
      case 'changes_requested':
        return 'Changes requested by the review team. Address the feedback and resubmit.';
      case 'pending':
        return 'Submitted for central review. You will be notified when reviewing is finished.';
      case 'draft':
        return 'Pushed but not yet submitted. Submit for review when you are ready to publish.';
      case 'rejected':
        return 'Rejected by the review team. Push a new version to try again.';
      case 'withdrawn':
        return 'Withdrawn from review. Submit it again whenever you are ready.';
      case 'approved':
        return null;
      default:
        throw new Error(`unhandled status: ${status satisfies never}`);
    }
  });
}
