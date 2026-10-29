import { atRoot, contextOf, inOrganization, organizationOf, withinOrganization } from './address';

describe('address', () => {
  it('reads the organization an address names', () => {
    expect(organizationOf('/organizations/gemeente-fundament/clusters')).toBe('gemeente-fundament');
  });

  it('names none for an address that carries none', () => {
    expect(organizationOf('/clusters')).toBeNull();
  });

  it('leaves the page behind once the organization comes off', () => {
    expect(
      withinOrganization('/organizations/gemeente-fundament/projects/pr-burgerzaken/members'),
    ).toBe('/projects/pr-burgerzaken/members');
  });

  it('leaves an address without an organization alone', () => {
    expect(withinOrganization('/clusters')).toBe('/clusters');
  });

  it('reads the organization itself as the empty page', () => {
    expect(withinOrganization('/organizations/gemeente-fundament')).toBe('/');
  });

  it('puts an address inside an organization', () => {
    expect(inOrganization('gemeente-fundament', '/clusters')).toBe(
      '/organizations/gemeente-fundament/clusters',
    );
  });

  it('turns the empty page into the address of the organization itself', () => {
    expect(inOrganization('gemeente-fundament', '/')).toBe('/organizations/gemeente-fundament');
  });

  it('moves an address that already names another organization', () => {
    expect(inOrganization('gemeente-delft', '/organizations/gemeente-fundament/clusters')).toBe(
      '/organizations/gemeente-delft/clusters',
    );
  });

  it('holds on to the query the presentation runs on', () => {
    expect(inOrganization('gemeente-fundament', '/clusters?present=1&slide=3')).toBe(
      '/organizations/gemeente-fundament/clusters?present=1&slide=3',
    );
  });

  it('keeps the organization itself without a slash before the query', () => {
    expect(inOrganization('gemeente-fundament', '/?present=1')).toBe(
      '/organizations/gemeente-fundament?present=1',
    );
  });

  it('leaves the address as it is while no organization is known yet', () => {
    expect(inOrganization(null, '/clusters')).toBe('/clusters');
  });

  it('reads the bare root as naming no page', () => {
    expect(atRoot('/')).toBe(true);
  });

  it('reads the root a visit brought a query to as naming no page', () => {
    expect(atRoot('/?present=1')).toBe(true);
    expect(atRoot('/#top')).toBe(true);
  });

  it('reads an address that names a page as naming one', () => {
    expect(atRoot('/clusters')).toBe(false);
    expect(atRoot('/organizations/gemeente-fundament')).toBe(false);
  });

  it('reads the cluster an address is in, and what is open under it', () => {
    expect(contextOf('/clusters/c1/nodes?tab=2')).toEqual({
      kind: 'clusters',
      id: 'c1',
      rest: '/nodes',
    });
    expect(contextOf('/projects/p1')).toEqual({ kind: 'projects', id: 'p1', rest: '' });
  });

  it('reads the new-project and new-cluster sheets as in no context', () => {
    expect(contextOf('/clusters/new')).toBeNull();
    expect(contextOf('/projects/new')).toBeNull();
    expect(contextOf('/clusters')).toBeNull();
  });
});
