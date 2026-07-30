// Error thrown by resource mutations that carries the HTTP status, so callers
// can distinguish e.g. a 403 (not permitted) from a transient failure.
export class KubeApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
  ) {
    super(message);
    this.name = 'KubeApiError';
  }
}

// Maps a delete failure to a user-facing message. A 403 comes from the
// user-credential path (the member's tenant SA RBAC denies delete), which is a
// permissions problem, not the transient failure "please try again" implies.
export function deleteErrorMessage(err: unknown): string {
  if (err instanceof KubeApiError && err.status === 403) {
    return 'You do not have permission to delete this resource.';
  }
  return 'Failed to delete. Please try again.';
}
