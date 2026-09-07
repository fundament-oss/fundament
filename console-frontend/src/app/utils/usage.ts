// Shared helpers for resource usage bars (metrics page, cluster details).

/**
 * Percentage of used over total, guarded so an absent total (metrics not
 * reported yet) renders as 0 instead of NaN.
 */
export function getUsagePercentage(used: number, total: number): number {
  if (total === 0) return 0;
  return Math.round((used / total) * 100);
}

/**
 * A usage figure as it is read out: whole numbers stay whole, anything else
 * keeps one decimal. Cluster CPU arrives as a float, so without this a bar
 * reads "2.4499999999999997 / 8 cores". Shared, so the metrics page and the
 * cluster page round the same number the same way.
 */
export function formatUsageValue(value: number): number {
  return Number.isInteger(value) ? value : Number(value.toFixed(1));
}

export function getUsageColor(percentage: number): string {
  if (percentage >= 90) return 'bg-danger-500';
  if (percentage >= 75) return 'bg-yellow-500';
  return 'bg-green-500';
}
