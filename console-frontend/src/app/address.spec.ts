import { inOrganization, organizationOf, withinOrganization } from './address';

describe('address', () => {
  it('reads the organization an address names', () => {
    expect(organizationOf('/organizations/gemeente-fundament/clusters')).toBe('gemeente-fundament');
  });

  it('names none for an address that carries none', () => {
    expect(organizationOf('/clusters')).toBeNull();
  });

  it('leaves the page behind once the organization comes off', () => {
    expect(
      withinOrganization('/organizations/gemeente-fundament/projects/pr-burgerzaken/limits'),
    ).toBe('/projects/pr-burgerzaken/limits');
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
});
