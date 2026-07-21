import {
  type SubmissionStatus,
  type RejectionReasonValue,
  REJECTION_REASONS,
} from './admin-review.service';

// Shared label + colour helpers so the queue and detail views render review
// status consistently, mirroring plugin-development/status-display.ts.

export const submissionStatusLabel = (status: SubmissionStatus): string => {
  switch (status) {
    case 'pending':
      return 'Pending review';
    case 'approved':
      return 'Approved';
    case 'rejected':
      return 'Rejected';
    default:
      throw new Error(`unhandled status: ${status satisfies never}`);
  }
};

export const submissionStatusTagColor = (
  status: SubmissionStatus,
): 'success' | 'warning' | 'critical' | 'neutral' => {
  switch (status) {
    case 'pending':
      return 'warning';
    case 'approved':
      return 'success';
    case 'rejected':
      return 'critical';
    default:
      throw new Error(`unhandled status: ${status satisfies never}`);
  }
};

// Class names for the `.badge` utility (see styles.css) used by the plain HTML
// submissions table. There is no red `.badge` variant, so rejected reuses the
// orange one, matching the warning/attention treatment used elsewhere.
export const submissionStatusBadgeClass = (status: SubmissionStatus): string => {
  switch (status) {
    case 'pending':
      return 'badge-blue';
    case 'approved':
      return 'badge-green';
    case 'rejected':
      return 'badge-orange';
    default:
      throw new Error(`unhandled status: ${status satisfies never}`);
  }
};

// Human-readable label for a stored rejection reason value.
export const rejectionReasonLabel = (value: RejectionReasonValue): string =>
  REJECTION_REASONS.find((reason) => reason.value === value)?.label ?? value;
