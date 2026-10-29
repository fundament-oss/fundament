import { convertToParamMap, UrlSegment } from '@angular/router';
import { DriftKind } from '../../generated/fleet/v1/fleet_pb';
import {
  findingPath,
  platformMatcher,
  platformPath,
  platformViewOf,
  type PlatformView,
} from './platform-views';

const segments = (path: string) =>
  path
    .split('/')
    .filter(Boolean)
    .map((part) => new UrlSegment(part, {}));

const paramsOf = (path: string) => {
  const match = platformMatcher(segments(path));
  if (!match) return null;
  return convertToParamMap(
    Object.fromEntries(
      Object.entries(match.posParams ?? {}).map(([key, segment]) => [key, segment.path]),
    ),
  );
};

describe('platformMatcher', () => {
  it('claims the four views', () => {
    [
      '/platform',
      '/platform/drift',
      '/platform/organizations/0198f3a1-7c2e-7b3a-9f1d-2c4e6a8b0d22',
      '/platform/clusters/0198f3a1-7c2e-7b3a-9f1d-2c4e6a8b0d1f',
    ].forEach((path) =>
      expect(platformMatcher(segments(path))?.consumed).toHaveLength(segments(path).length),
    );
  });

  it('leaves other sections and unknown shapes alone', () => {
    expect(platformMatcher(segments('/racks'))).toBeNull();
    expect(platformMatcher(segments('/platform/organizations'))).toBeNull();
    expect(platformMatcher(segments('/platform/nodes/x'))).toBeNull();
    expect(platformMatcher(segments('/platform/clusters/x/extra'))).toBeNull();
  });
});

describe('platformViewOf', () => {
  it('reads every view back from the matched parameters', () => {
    expect(platformViewOf(paramsOf('/platform')!)).toEqual({ kind: 'overview' });
    expect(platformViewOf(paramsOf('/platform/drift')!)).toEqual({ kind: 'drift' });
    expect(platformViewOf(paramsOf('/platform/organizations/o1')!)).toEqual({
      kind: 'organization',
      id: 'o1',
    });
    expect(platformViewOf(paramsOf('/platform/clusters/c1')!)).toEqual({
      kind: 'cluster',
      id: 'c1',
    });
  });

  it('is the inverse of platformPath', () => {
    const views: PlatformView[] = [
      { kind: 'overview' },
      { kind: 'drift' },
      { kind: 'organization', id: 'o1' },
      { kind: 'cluster', id: 'c1' },
    ];
    views.forEach((view) => expect(platformViewOf(paramsOf(platformPath(view))!)).toEqual(view));
  });
});

describe('findingPath', () => {
  it('links a finding to its cluster only while that cluster is live', () => {
    [DriftKind.CLUSTER_WITHOUT_SHOOT, DriftKind.KUBERNETES_VERSION_MISMATCH].forEach((kind) =>
      expect(findingPath({ kind, clusterId: 'c1' })).toBe('/platform/clusters/c1'),
    );
    expect(findingPath({ kind: DriftKind.SHOOT_OF_DELETED_CLUSTER, clusterId: 'c1' })).toBeNull();
    expect(findingPath({ kind: DriftKind.SHOOT_WITHOUT_CLUSTER, clusterId: 'c1' })).toBeNull();
    expect(findingPath({ kind: DriftKind.SHOOT_INVALID_CLUSTER_ID, clusterId: '' })).toBeNull();
  });
});
