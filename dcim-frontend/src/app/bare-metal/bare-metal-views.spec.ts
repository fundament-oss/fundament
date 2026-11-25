import { convertToParamMap, UrlSegment } from '@angular/router';
import {
  bareMetalMatcher,
  bareMetalPath,
  bareMetalViewOf,
  type BareMetalView,
} from './bare-metal-views';

const segments = (path: string) =>
  path
    .split('/')
    .filter(Boolean)
    .map((part) => new UrlSegment(part, {}));

const paramsOf = (path: string) => {
  const match = bareMetalMatcher(segments(path));
  if (!match) return null;
  return convertToParamMap(
    Object.fromEntries(
      Object.entries(match.posParams ?? {}).map(([key, segment]) => [key, segment.path]),
    ),
  );
};

describe('bareMetalMatcher', () => {
  it('claims the section and its four views', () => {
    [
      '/bare-metal',
      '/bare-metal/capacity',
      '/bare-metal/machines',
      '/bare-metal/network',
      '/bare-metal/api',
    ].forEach((path) =>
      expect(bareMetalMatcher(segments(path))?.consumed).toHaveLength(segments(path).length),
    );
  });

  it('leaves other sections and unknown shapes alone', () => {
    expect(bareMetalMatcher(segments('/racks'))).toBeNull();
    expect(bareMetalMatcher(segments('/bare-metal/unknown'))).toBeNull();
    expect(bareMetalMatcher(segments('/bare-metal/machines/extra'))).toBeNull();
    expect(bareMetalMatcher(segments('/platform'))).toBeNull();
  });
});

describe('bareMetalViewOf', () => {
  it('reads every view back from the matched parameters', () => {
    expect(bareMetalViewOf(paramsOf('/bare-metal')!)).toEqual({ kind: 'capacity' });
    expect(bareMetalViewOf(paramsOf('/bare-metal/capacity')!)).toEqual({ kind: 'capacity' });
    expect(bareMetalViewOf(paramsOf('/bare-metal/machines')!)).toEqual({ kind: 'machines' });
    expect(bareMetalViewOf(paramsOf('/bare-metal/network')!)).toEqual({ kind: 'network' });
    expect(bareMetalViewOf(paramsOf('/bare-metal/api')!)).toEqual({ kind: 'api' });
  });

  it('is the inverse of bareMetalPath', () => {
    const views: BareMetalView[] = [
      { kind: 'capacity' },
      { kind: 'machines' },
      { kind: 'network' },
      { kind: 'api' },
    ];
    views.forEach((view) => expect(bareMetalViewOf(paramsOf(bareMetalPath(view))!)).toEqual(view));
  });

  it('gives every view an address below the section, so the shell opens it as depth 2', () => {
    expect(bareMetalPath({ kind: 'capacity' })).toBe('/bare-metal/capacity');
  });
});
