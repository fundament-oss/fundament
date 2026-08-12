import { inOrganization, organizationOf, withinOrganization } from './address';

describe('address', () => {
  it('leest de organisatie die een adres noemt', () => {
    expect(organizationOf('/organizations/gemeente-fundament/clusters')).toBe('gemeente-fundament');
  });

  it('noemt er geen bij een adres dat er geen draagt', () => {
    expect(organizationOf('/clusters')).toBeNull();
  });

  it('houdt de pagina over als de organisatie eraf gaat', () => {
    expect(
      withinOrganization('/organizations/gemeente-fundament/projects/pr-burgerzaken/limits'),
    ).toBe('/projects/pr-burgerzaken/limits');
  });

  it('laat een adres zonder organisatie ongemoeid', () => {
    expect(withinOrganization('/clusters')).toBe('/clusters');
  });

  it('leest de organisatie zelf als de lege pagina', () => {
    expect(withinOrganization('/organizations/gemeente-fundament')).toBe('/');
  });

  it('zet een adres in een organisatie', () => {
    expect(inOrganization('gemeente-fundament', '/clusters')).toBe(
      '/organizations/gemeente-fundament/clusters',
    );
  });

  it('maakt van de lege pagina het adres van de organisatie zelf', () => {
    expect(inOrganization('gemeente-fundament', '/')).toBe('/organizations/gemeente-fundament');
  });

  it('verhuist een adres dat al een andere organisatie noemt', () => {
    expect(inOrganization('gemeente-delft', '/organizations/gemeente-fundament/clusters')).toBe(
      '/organizations/gemeente-delft/clusters',
    );
  });

  it('houdt de queryparameters vast, waar de presentatie op draait', () => {
    expect(inOrganization('gemeente-fundament', '/clusters?present=1&slide=3')).toBe(
      '/organizations/gemeente-fundament/clusters?present=1&slide=3',
    );
  });

  it('houdt de organisatie zelf zonder slash voor de queryparameters', () => {
    expect(inOrganization('gemeente-fundament', '/?present=1')).toBe(
      '/organizations/gemeente-fundament?present=1',
    );
  });

  it('laat het adres staan zolang er nog geen organisatie bekend is', () => {
    expect(inOrganization(null, '/clusters')).toBe('/clusters');
  });
});
