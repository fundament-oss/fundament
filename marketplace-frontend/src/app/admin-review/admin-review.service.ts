import { Injectable } from '@angular/core';

// Review status of a plugin submission as it moves through the admin queue.
export type SubmissionStatus = 'pending' | 'approved' | 'rejected';

export interface Submitter {
  name: string;
  email: string;
  team: string;
}

// A plugin submission awaiting (or having gone through) admin review. For now a
// submission is just the metadata an author fills in: title, description and
// one or more categories.
export interface PluginSubmission {
  id: string; // stable identifier, used in URLs
  title: string;
  description: string;
  categories: string[];
  icon: string; // base name under /img/plugins/<icon>.svg
  submitter: Submitter;
  submittedAt: string; // ISO date
  status: SubmissionStatus;
  // Set once a decision is made.
  reviewedAt?: string; // ISO date
  reviewedBy?: string;
  // Present only when status === 'rejected'.
  rejectionReason?: RejectionReasonValue;
  feedback?: string;
}

// Fixed list of rejection reasons offered in the reject dialog dropdown.
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

export interface RejectDecision {
  reason: RejectionReasonValue;
  feedback?: string;
}

// Hardcoded mock data. This service intentionally mimics the shape of the
// ConnectRPC-backed services (async methods returning promises) so it can be
// swapped for a real admin review API later without touching the components.
// approve()/reject() mutate the in-memory array so decisions persist for the
// rest of the session (e.g. navigating back to the queue reflects them).
const MOCK_SUBMISSIONS: PluginSubmission[] = [
  {
    id: 'redis-operator',
    title: 'Redis Operator',
    description:
      'Provision and operate managed Redis instances on your clusters, with automated failover, persistence and TLS. Exposes a Redis custom resource so teams can self-serve caches and queues.',
    categories: ['Database', 'Caching'],
    icon: 'cloudnativepg',
    submitter: { name: 'Platform Data Team', email: 'data@example.gov', team: 'Data' },
    submittedAt: '2026-07-19',
    status: 'pending',
  },
  {
    id: 'audit-logger',
    title: 'Audit Logger',
    description:
      'Streams Kubernetes audit events to a central, tamper-evident store and ships a set of compliance dashboards. Helps meet retention requirements for government workloads.',
    categories: ['Security', 'Observability'],
    icon: 'grafana-loki',
    submitter: { name: 'Security Guild', email: 'security@example.gov', team: 'Security' },
    submittedAt: '2026-07-18',
    status: 'pending',
  },
  {
    id: 'cost-insights',
    title: 'Cost Insights',
    description:
      'Breaks down cluster spend by namespace, team and workload, with budget alerts and idle-resource recommendations. Integrates with the platform billing export.',
    categories: ['Observability', 'FinOps'],
    icon: 'grafana-mimir',
    submitter: { name: 'Platform Team', email: 'platform@example.gov', team: 'Platform' },
    submittedAt: '2026-07-16',
    status: 'pending',
  },
  {
    id: 'ingress-shield',
    title: 'Ingress Shield',
    description:
      'A web application firewall and rate limiter for ingress traffic, with managed rule sets and per-route policies. Sits in front of your existing ingress controller.',
    categories: ['Security', 'Networking'],
    icon: 'istio',
    submitter: { name: 'Networking Crew', email: 'net@example.gov', team: 'Networking' },
    submittedAt: '2026-07-14',
    status: 'approved',
    reviewedAt: '2026-07-15',
    reviewedBy: 'a.jansen@example.gov',
  },
  {
    id: 'quick-backup',
    title: 'Quick Backup',
    description:
      'One-click volume snapshots for stateful workloads, stored in object storage. Restore is manual via the CLI.',
    categories: ['Storage'],
    icon: 'sealed-secrets',
    submitter: { name: 'Internal Tooling', email: 'tooling@example.gov', team: 'Tooling' },
    submittedAt: '2026-07-11',
    status: 'rejected',
    reviewedAt: '2026-07-12',
    reviewedBy: 'a.jansen@example.gov',
    rejectionReason: 'security_concerns',
    feedback:
      'The backup controller requests cluster-admin. Please scope its ServiceAccount down to the namespaces it needs and resubmit.',
  },
];

// Stand-in for the signed-in admin in this mockup.
const REVIEWER_EMAIL = 'a.jansen@example.gov';

const todayIso = (): string => new Date().toISOString().slice(0, 10);

@Injectable({ providedIn: 'root' })
export default class AdminReviewService {
  private readonly submissions = MOCK_SUBMISSIONS;

  listSubmissions(): Promise<PluginSubmission[]> {
    return Promise.resolve(this.submissions.map((submission) => ({ ...submission })));
  }

  getSubmission(id: string): Promise<PluginSubmission | null> {
    const submission = this.submissions.find((s) => s.id === id);
    return Promise.resolve(submission ? { ...submission } : null);
  }

  approve(id: string): Promise<void> {
    const submission = this.submissions.find((s) => s.id === id);
    if (submission) {
      submission.status = 'approved';
      submission.reviewedAt = todayIso();
      submission.reviewedBy = REVIEWER_EMAIL;
      submission.rejectionReason = undefined;
      submission.feedback = undefined;
    }
    return Promise.resolve();
  }

  reject(id: string, decision: RejectDecision): Promise<void> {
    const submission = this.submissions.find((s) => s.id === id);
    if (submission) {
      submission.status = 'rejected';
      submission.reviewedAt = todayIso();
      submission.reviewedBy = REVIEWER_EMAIL;
      submission.rejectionReason = decision.reason;
      submission.feedback = decision.feedback?.trim() || undefined;
    }
    return Promise.resolve();
  }
}
