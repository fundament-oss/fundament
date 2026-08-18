// Shared helpers for resource usage bars (metrics page, cluster details).

/**
 * Percentage of used over total, guarded so an absent total (metrics not
 * reported yet) renders as 0 instead of NaN.
 */
export function getUsagePercentage(used: number, total: number): number {
  if (total === 0) return 0;
  return Math.round((used / total) * 100);
}

export function getUsageColor(percentage: number): string {
  if (percentage >= 90) return 'bg-danger-500';
  if (percentage >= 75) return 'bg-yellow-500';
  return 'bg-green-500';
}
