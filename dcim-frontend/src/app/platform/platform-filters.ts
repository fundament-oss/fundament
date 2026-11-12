import {
  type ClusterStatus,
  type ClusterSummary,
  type DriftFinding,
  type DriftKind,
  type OrganizationSummary,
} from '../../generated/fleet/v1/fleet_pb';
import { clusterTag, driftKindTag } from './fleet-format';

/** A filter that is off: every row passes. */
export const ALL = 'all';

/** One choice in a filter menu, with how many of the rows it would leave. */
export interface FilterOption<T> {
  value: T;
  label: string;
  count: number;
}

/** True when every word of the query is in one of the fields, ignoring case. */
export function matchesQuery(query: string, ...fields: (string | undefined)[]): boolean {
  const words = query.trim().toLowerCase().split(/\s+/).filter(Boolean);
  if (words.length === 0) return true;
  const haystack = fields
    .filter((field): field is string => !!field)
    .join(' ')
    .toLowerCase();
  return words.every((word) => haystack.includes(word));
}

export function filterOrganizations(
  organizations: OrganizationSummary[],
  query: string,
  unhealthyOnly: boolean,
): OrganizationSummary[] {
  return organizations.filter(
    (organization) =>
      (!unhealthyOnly || organization.clustersUnhealthy > 0) &&
      matchesQuery(query, organization.alias, organization.name),
  );
}

export function filterClusters(
  clusters: ClusterSummary[],
  query: string,
  status: ClusterStatus | typeof ALL,
): ClusterSummary[] {
  return clusters.filter(
    (cluster) =>
      (status === ALL || cluster.status === status) &&
      matchesQuery(query, cluster.name, cluster.region, cluster.kubernetesVersion),
  );
}

export function filterFindings(
  findings: DriftFinding[],
  query: string,
  kind: DriftKind | typeof ALL,
): DriftFinding[] {
  return findings.filter(
    (finding) =>
      (kind === ALL || finding.kind === kind) &&
      matchesQuery(
        query,
        finding.organizationName,
        finding.clusterName,
        finding.clusterId,
        finding.shootNamespace,
        finding.shootName,
        finding.detail,
      ),
  );
}

/**
 * The values that occur in the rows, in enum order, so the menu offers only
 * choices that leave something. The current pick stays in the menu even when
 * a refresh took its last row away, or it could not be seen or undone.
 */
function optionsOf<T extends number>(
  values: T[],
  selected: T | typeof ALL,
  label: (value: T) => string,
): FilterOption<T>[] {
  const counts = new Map<T, number>();
  values.forEach((value) => counts.set(value, (counts.get(value) ?? 0) + 1));
  if (selected !== ALL && !counts.has(selected)) counts.set(selected, 0);
  return [...counts.entries()]
    .sort(([a], [b]) => a - b)
    .map(([value, count]) => ({ value, label: label(value), count }));
}

export function clusterStatusOptions(
  clusters: ClusterSummary[],
  selected: ClusterStatus | typeof ALL,
): FilterOption<ClusterStatus>[] {
  return optionsOf(
    clusters.map((cluster) => cluster.status),
    selected,
    (status) => clusterTag({ status }).text,
  );
}

export function driftKindOptions(
  findings: DriftFinding[],
  selected: DriftKind | typeof ALL,
): FilterOption<DriftKind>[] {
  return optionsOf(
    findings.map((finding) => finding.kind),
    selected,
    (kind) => driftKindTag(kind).text,
  );
}

/** "3 of 12 clusters" while something narrows the list, "12 clusters" when nothing does. */
export function countText(shown: number, total: number, singular: string, plural: string): string {
  const noun = total === 1 ? singular : plural;
  return shown === total ? `${total} ${noun}` : `${shown} of ${total} ${noun}`;
}
