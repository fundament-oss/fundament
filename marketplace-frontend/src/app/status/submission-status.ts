import { SubmissionStatus as ProtoSubmissionStatus } from '../../generated/marketplace/v1/common_pb';
import { RejectionReason as ProtoRejectionReason } from '../../generated/admin/v1/common_pb';

// One status vocabulary for the developer and the reviewer alike (FUN-20): they
// are looking at the same version in the same pipeline, so it does not get two
// sets of names depending on which surface you ask.
//
// draft              pushed via functl, not yet submitted; no submission exists
// pending            submitted, awaiting a decision
// changes_requested  returned with a note; the developer can fix and resubmit
// approved           approved and live in the catalog
// rejected           refused; the submission is closed
// withdrawn          pulled back before a decision; can be submitted again
export type SubmissionStatus =
  'draft' | 'pending' | 'changes_requested' | 'approved' | 'rejected' | 'withdrawn';

const FROM_PROTO: Record<ProtoSubmissionStatus, SubmissionStatus> = {
  // A version always has a status; UNSPECIFIED would be a server bug, and draft
  // is the state that claims the least.
  [ProtoSubmissionStatus.UNSPECIFIED]: 'draft',
  [ProtoSubmissionStatus.DRAFT]: 'draft',
  [ProtoSubmissionStatus.PENDING]: 'pending',
  [ProtoSubmissionStatus.CHANGES_REQUESTED]: 'changes_requested',
  [ProtoSubmissionStatus.APPROVED]: 'approved',
  [ProtoSubmissionStatus.REJECTED]: 'rejected',
  [ProtoSubmissionStatus.WITHDRAWN]: 'withdrawn',
};

const TO_PROTO: Record<SubmissionStatus, ProtoSubmissionStatus> = {
  draft: ProtoSubmissionStatus.DRAFT,
  pending: ProtoSubmissionStatus.PENDING,
  changes_requested: ProtoSubmissionStatus.CHANGES_REQUESTED,
  approved: ProtoSubmissionStatus.APPROVED,
  rejected: ProtoSubmissionStatus.REJECTED,
  withdrawn: ProtoSubmissionStatus.WITHDRAWN,
};

export const fromProtoStatus = (status: ProtoSubmissionStatus): SubmissionStatus =>
  FROM_PROTO[status] ?? 'draft';

export const toProtoStatus = (status: SubmissionStatus): ProtoSubmissionStatus => TO_PROTO[status];

export const statusLabel = (status: SubmissionStatus): string => {
  switch (status) {
    case 'draft':
      return 'Draft';
    case 'pending':
      return 'Pending review';
    case 'changes_requested':
      return 'Changes requested';
    case 'approved':
      return 'Approved';
    case 'rejected':
      return 'Rejected';
    case 'withdrawn':
      return 'Withdrawn';
    default:
      throw new Error(`unhandled status: ${status satisfies never}`);
  }
};

export const statusTagColor = (
  status: SubmissionStatus,
): 'success' | 'accent' | 'warning' | 'critical' | 'neutral' => {
  switch (status) {
    case 'draft':
      return 'neutral';
    case 'pending':
      return 'accent';
    case 'changes_requested':
      return 'warning';
    case 'approved':
      return 'success';
    case 'rejected':
      return 'critical';
    case 'withdrawn':
      return 'neutral';
    default:
      throw new Error(`unhandled status: ${status satisfies never}`);
  }
};

// Class names for the `.badge` utility (see styles.css) used by the plain HTML
// tables, as an alternative to <nldd-tag> for those views. There is no red
// `.badge` variant, so rejected reuses the orange one.
export const statusBadgeClass = (status: SubmissionStatus): string => {
  switch (status) {
    case 'draft':
      return 'badge-gray';
    case 'pending':
      return 'badge-blue';
    case 'changes_requested':
      return 'badge-orange';
    case 'approved':
      return 'badge-green';
    case 'rejected':
      return 'badge-orange';
    case 'withdrawn':
      return 'badge-gray';
    default:
      throw new Error(`unhandled status: ${status satisfies never}`);
  }
};

// Fixed list of rejection reasons offered in the reject dialog dropdown.
// Backoffice reporting only: the developer never sees the reason itself, they
// receive the reviewer's feedback.
export type RejectionReasonValue =
  | 'incomplete_metadata'
  | 'duplicate'
  | 'security_concerns'
  | 'naming_guidelines'
  | 'out_of_scope'
  | 'other';

export interface RejectionReason {
  value: RejectionReasonValue;
  label: string;
}

export const REJECTION_REASONS: RejectionReason[] = [
  { value: 'incomplete_metadata', label: 'Incomplete or unclear metadata' },
  { value: 'duplicate', label: 'Duplicate of an existing plugin' },
  { value: 'security_concerns', label: 'Security or permission concerns' },
  { value: 'naming_guidelines', label: 'Does not meet naming guidelines' },
  { value: 'out_of_scope', label: 'Out of scope for the marketplace' },
  { value: 'other', label: 'Other (see feedback)' },
];

const TO_PROTO_REASON: Record<RejectionReasonValue, ProtoRejectionReason> = {
  incomplete_metadata: ProtoRejectionReason.INCOMPLETE_METADATA,
  duplicate: ProtoRejectionReason.DUPLICATE,
  security_concerns: ProtoRejectionReason.SECURITY_CONCERNS,
  naming_guidelines: ProtoRejectionReason.NAMING_GUIDELINES,
  out_of_scope: ProtoRejectionReason.OUT_OF_SCOPE,
  other: ProtoRejectionReason.OTHER,
};

const FROM_PROTO_REASON: Record<ProtoRejectionReason, RejectionReasonValue | undefined> = {
  // RejectionReason is only set once a version is rejected; UNSPECIFIED means
  // "no decision recorded", not a reason of its own.
  [ProtoRejectionReason.UNSPECIFIED]: undefined,
  [ProtoRejectionReason.INCOMPLETE_METADATA]: 'incomplete_metadata',
  [ProtoRejectionReason.DUPLICATE]: 'duplicate',
  [ProtoRejectionReason.SECURITY_CONCERNS]: 'security_concerns',
  [ProtoRejectionReason.NAMING_GUIDELINES]: 'naming_guidelines',
  [ProtoRejectionReason.OUT_OF_SCOPE]: 'out_of_scope',
  [ProtoRejectionReason.OTHER]: 'other',
};

export const toProtoRejectionReason = (value: RejectionReasonValue): ProtoRejectionReason =>
  TO_PROTO_REASON[value];

export const fromProtoRejectionReason = (
  reason: ProtoRejectionReason,
): RejectionReasonValue | undefined => FROM_PROTO_REASON[reason];

// Human-readable label for a stored rejection reason value.
export const rejectionReasonLabel = (value: RejectionReasonValue): string =>
  REJECTION_REASONS.find((reason) => reason.value === value)?.label ?? value;
