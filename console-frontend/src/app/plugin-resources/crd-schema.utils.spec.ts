import { fieldNameToLabel, kindToLabel } from './crd-schema.utils';

describe('kindToLabel', () => {
  it('pluralizes a single-word kind', () => {
    expect(kindToLabel('Certificate')).toBe('Certificates');
  });

  it('splits PascalCase into sentence case', () => {
    expect(kindToLabel('ClusterIssuer')).toBe('Cluster issuers');
    expect(kindToLabel('CertificateRequest')).toBe('Certificate requests');
  });

  it('keeps a leading acronym intact', () => {
    // Splitting on every capital gave "F s c installations".
    expect(kindToLabel('FSCInstallation')).toBe('FSC installations');
  });

  it('keeps an acronym intact wherever it appears', () => {
    expect(kindToLabel('HTTPRoute')).toBe('HTTP routes');
    expect(kindToLabel('ClusterHTTPRoute')).toBe('Cluster HTTP routes');
  });

  it('applies the pluralization rules to the last word', () => {
    expect(kindToLabel('NetworkPolicy')).toBe('Network policies');
    expect(kindToLabel('IngressClass')).toBe('Ingress classes');
  });

  it('returns an empty string for an empty kind', () => {
    expect(kindToLabel('')).toBe('');
  });
});

describe('fieldNameToLabel', () => {
  it('capitalizes a single word', () => {
    expect(fieldNameToLabel('namespace')).toBe('Namespace');
  });

  it('splits camelCase', () => {
    expect(fieldNameToLabel('selfAddress')).toBe('Self Address');
    expect(fieldNameToLabel('autoSignGrants')).toBe('Auto Sign Grants');
  });

  it('keeps a trailing acronym intact', () => {
    // Splitting on every capital gave "Peer I D" / "Controller U R L".
    expect(fieldNameToLabel('peerID')).toBe('Peer ID');
    expect(fieldNameToLabel('groupID')).toBe('Group ID');
    expect(fieldNameToLabel('controllerURL')).toBe('Controller URL');
  });

  it('keeps an acronym intact mid-name', () => {
    expect(fieldNameToLabel('tlsCABundle')).toBe('Tls CA Bundle');
  });

  it('returns an empty string for an empty name', () => {
    expect(fieldNameToLabel('')).toBe('');
  });
});
