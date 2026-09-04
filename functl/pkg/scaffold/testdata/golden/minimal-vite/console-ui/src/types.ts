// The shapes this plugin's views read off its custom resource.
// TODO: replace these with the real fields of widgets.example.com.

export interface Condition {
  type: string;
  status: string;
  reason?: string;
  message?: string;
  lastTransitionTime?: string;
}

export interface ObjectMeta {
  name?: string;
  namespace?: string;
  creationTimestamp?: string;
}

export interface Widget {
  metadata?: ObjectMeta;
  spec?: Record<string, unknown>;
  status?: {
    phase?: string;
    conditions?: Condition[];
  };
}
