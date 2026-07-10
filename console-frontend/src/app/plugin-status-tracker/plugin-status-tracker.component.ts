import { Component, input, computed, ChangeDetectionStrategy } from '@angular/core';
import { CheckmarkIconComponent } from '../icons';
import { type PluginStatus } from '../plugin-development/plugin-development.service';

type StepState = 'complete' | 'active' | 'error' | 'upcoming';

interface TrackerStep {
  name: string;
  state: StepState;
}

// Read-only status indicator for the plugin publishing pipeline:
// Pushed via functl -> Central review -> Publish. Non-navigable; the visual
// style is adapted from the add-cluster wizard's progress bar.
@Component({
  selector: 'app-plugin-status-tracker',
  imports: [CheckmarkIconComponent],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './plugin-status-tracker.component.html',
})
export default class PluginStatusTrackerComponent {
  status = input.required<PluginStatus>();

  steps = computed<TrackerStep[]>(() => {
    const reviewState = PluginStatusTrackerComponent.reviewState(this.status());
    const publishState: StepState = this.status() === 'published' ? 'complete' : 'upcoming';

    return [
      { name: 'Pushed via functl', state: 'complete' },
      { name: 'Central review', state: reviewState },
      { name: 'Publish', state: publishState },
    ];
  });

  private static reviewState(status: PluginStatus): StepState {
    switch (status) {
      case 'published':
        return 'complete';
      case 'changes_requested':
        return 'error';
      case 'in_review':
        return 'active';
      default:
        return 'upcoming';
    }
  }

  // Short hint shown under the tracker when the plugin needs author action.
  hint = computed<string | null>(() => {
    switch (this.status()) {
      case 'changes_requested':
        return 'Changes requested by the review team. Push a new version to resubmit.';
      case 'in_review':
        return 'Submitted for central review. You will be notified when reviewing is finished.';
      case 'pushed':
        return 'Pushed but not yet submitted. Submit for review when you are ready to publish.';
      default:
        return null;
    }
  });
}
