import { type ParamMap, UrlSegment, type UrlMatchResult } from '@angular/router';
import { DriftKind, type DriftFinding } from '../../generated/fleet/v1/fleet_pb';

/** The section's own address. Every view of the fleet lives under it. */
export const PLATFORM_PATH = '/platform';

/** What the Platform section can show: the whole fleet, one organization, one
 *  cluster, or what disagrees with Gardener. */
export type PlatformView =
  | { kind: 'overview' }
  | { kind: 'organization'; id: string }
  | { kind: 'cluster'; id: string }
  | { kind: 'drift' };

/**
 * /platform, /platform/drift, /platform/organizations/<id> and
 * /platform/clusters/<id>: one route config for all four, as the racks section
 * does, so moving between them is a parameter change on the same page and the
 * menu beside it keeps what it loaded.
 */
export function platformMatcher(segments: UrlSegment[]): UrlMatchResult | null {
  if (segments.length === 0 || segments[0].path !== 'platform') return null;
  if (segments.length === 1) return { consumed: segments, posParams: {} };
  if (segments.length === 2 && segments[1].path === 'drift') {
    return { consumed: segments, posParams: { view: segments[1] } };
  }
  if (
    segments.length === 3 &&
    (segments[1].path === 'organizations' || segments[1].path === 'clusters')
  ) {
    return { consumed: segments, posParams: { view: segments[1], id: segments[2] } };
  }
  return null;
}

/** Reads the view back from the parameters the matcher set. */
export function platformViewOf(params: ParamMap): PlatformView {
  const view = params.get('view');
  const id = params.get('id');
  if (view === 'drift') return { kind: 'drift' };
  if (view === 'organizations' && id) return { kind: 'organization', id };
  if (view === 'clusters' && id) return { kind: 'cluster', id };
  return { kind: 'overview' };
}

/** The address of a view, the inverse of the matcher. */
export function platformPath(view: PlatformView): string {
  switch (view.kind) {
    case 'overview':
      return PLATFORM_PATH;
    case 'drift':
      return `${PLATFORM_PATH}/drift`;
    case 'organization':
      return `${PLATFORM_PATH}/organizations/${view.id}`;
    case 'cluster':
      return `${PLATFORM_PATH}/clusters/${view.id}`;
    default: {
      const unhandled: never = view;
      throw new Error(`unhandled platform view ${JSON.stringify(unhandled)}`);
    }
  }
}

export const organizationPath = (id: string): string => platformPath({ kind: 'organization', id });

export const clusterPath = (id: string): string => platformPath({ kind: 'cluster', id });

/** Where a drift finding leads: its cluster, while that is a live one. */
export function findingPath(finding: Pick<DriftFinding, 'kind' | 'clusterId'>): string | null {
  switch (finding.kind) {
    case DriftKind.CLUSTER_WITHOUT_SHOOT:
    case DriftKind.SNAPSHOT_STALE:
    case DriftKind.KUBERNETES_VERSION_MISMATCH:
      return clusterPath(finding.clusterId);
    case DriftKind.SHOOT_WITHOUT_CLUSTER:
    case DriftKind.SHOOT_OF_DELETED_CLUSTER:
    case DriftKind.UNSPECIFIED:
      return null;
    default:
      throw new Error(`unhandled drift kind ${finding.kind satisfies never}`);
  }
}
