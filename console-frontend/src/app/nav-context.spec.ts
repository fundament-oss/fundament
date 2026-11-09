import { describe, expect, it } from 'vitest';
import {
  contextOf,
  contextRoot,
  depthForPath,
  isContextRoot,
  overviewOf,
  parentOf,
} from './nav-context';

const ORG = '/organizations/acme-corp';

describe('contextOf', () => {
  it('reads the organization from its own pages', () => {
    expect(contextOf(ORG)).toEqual({ kind: 'organization' });
    expect(contextOf(`${ORG}/members`)).toEqual({ kind: 'organization' });
    expect(contextOf(`${ORG}/clusters`)).toEqual({ kind: 'organization' });
  });

  it('reads a cluster from its address and every page under it', () => {
    expect(contextOf(`${ORG}/clusters/c1`)).toEqual({ kind: 'cluster', clusterId: 'c1' });
    expect(contextOf(`${ORG}/clusters/c1/general/nodes`)).toEqual({
      kind: 'cluster',
      clusterId: 'c1',
    });
    expect(contextOf(`${ORG}/clusters/c1/plugin-resources/p/issuers?x=1`)).toEqual({
      kind: 'cluster',
      clusterId: 'c1',
    });
  });

  it('reads a project from its address and every page under it', () => {
    expect(contextOf(`${ORG}/projects/p1`)).toEqual({ kind: 'project', projectId: 'p1' });
    expect(contextOf(`${ORG}/projects/p1/members/m1#top`)).toEqual({
      kind: 'project',
      projectId: 'p1',
    });
  });

  it('keeps the create sheets in the organization', () => {
    expect(contextOf(`${ORG}/clusters/new`)).toEqual({ kind: 'organization' });
    expect(contextOf(`${ORG}/projects/new`)).toEqual({ kind: 'organization' });
  });

  it('reads an address written without the organization the same way', () => {
    expect(contextOf('/projects/p1/general')).toEqual({ kind: 'project', projectId: 'p1' });
  });
});

describe('contextRoot', () => {
  it('is the address of the place itself', () => {
    expect(contextRoot({ kind: 'organization' })).toBe('/');
    expect(contextRoot({ kind: 'cluster', clusterId: 'c1' })).toBe('/clusters/c1');
    expect(contextRoot({ kind: 'project', projectId: 'p1' })).toBe('/projects/p1');
  });
});

describe('overviewOf', () => {
  it('opens the overview page of each kind of place', () => {
    expect(overviewOf({ kind: 'organization' })).toBe('/general');
    expect(overviewOf({ kind: 'cluster', clusterId: 'c1' })).toBe('/clusters/c1/general');
    expect(overviewOf({ kind: 'project', projectId: 'p1' })).toBe('/projects/p1/general');
  });

  it('is a page inside its place, not the place itself', () => {
    expect(isContextRoot(overviewOf({ kind: 'cluster', clusterId: 'c1' }))).toBe(false);
  });
});

describe('isContextRoot and depthForPath', () => {
  it('shows the menu on a place itself', () => {
    [ORG, `${ORG}?present=1`, `${ORG}/clusters/c1`, `${ORG}/projects/p1`].forEach((url) => {
      expect(isContextRoot(url), url).toBe(true);
      expect(depthForPath(url), url).toBe(0);
    });
  });

  it('shows the page on anything inside one', () => {
    [
      `${ORG}/clusters`,
      `${ORG}/clusters/new`,
      `${ORG}/projects/new`,
      `${ORG}/clusters/c1/general`,
      `${ORG}/clusters/c1/projects`,
      `${ORG}/projects/p1/namespaces`,
    ].forEach((url) => {
      expect(isContextRoot(url), url).toBe(false);
      expect(depthForPath(url), url).toBe(1);
    });
  });
});

describe('parentOf', () => {
  it('leads a page to the menu of the place it is in', () => {
    expect(parentOf(`${ORG}/clusters/c1/plugin-resources/p/Kind/x?a=b`)).toBe('/clusters/c1');
    expect(parentOf(`${ORG}/clusters/c1/general`)).toBe('/clusters/c1');
    expect(parentOf(`${ORG}/projects/p1/members`)).toBe('/projects/p1');
    expect(parentOf(`${ORG}/clusters`)).toBe('/');
  });
});
