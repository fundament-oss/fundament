import {
  crdRefToLabel,
  fieldNameToLabel,
  kindToLabel,
  kindToSingularLabel,
  labelMidSentence,
} from './crd-schema.utils';

describe('kindToLabel', () => {
  it('pluralizes a single-word kind', () => {
    expect(kindToLabel('Certificate')).toBe('Certificates');
  });

  it('splits PascalCase on every word boundary', () => {
    expect(kindToLabel('ClusterIssuer')).toBe('Cluster Issuers');
    expect(kindToLabel('CertificateRequest')).toBe('Certificate Requests');
  });

  it('keeps a leading acronym intact', () => {
    // Splitting on every capital gave "F S C Installations"; not splitting at all
    // gave "Fscinstallations".
    expect(kindToLabel('FSCInstallation')).toBe('FSC Installations');
    expect(kindToLabel('DNSEndpoint')).toBe('DNS Endpoints');
  });

  it('keeps an acronym intact wherever it appears', () => {
    expect(kindToLabel('HTTPRoute')).toBe('HTTP Routes');
    expect(kindToLabel('ClusterHTTPRoute')).toBe('Cluster HTTP Routes');
    expect(kindToLabel('MyXYZ')).toBe('My XYZs');
  });

  it('applies the pluralization rules to the last word', () => {
    expect(kindToLabel('NetworkPolicy')).toBe('Network Policies');
    expect(kindToLabel('IngressClass')).toBe('Ingress Classes');
  });

  it('keeps a trailing acronym intact once pluralized', () => {
    // Pluralizing first gave "HTTPs", which no longer reads as an acronym, so
    // deciding the case afterwards wrote "Cluster Https".
    expect(kindToLabel('ClusterHTTP')).toBe('Cluster HTTPs');
    expect(kindToLabel('FSC')).toBe('FSCs');
  });

  it('returns an empty string for an empty kind', () => {
    expect(kindToLabel('')).toBe('');
  });
});

describe('kindToSingularLabel', () => {
  it('keeps the kind singular, split on every word boundary', () => {
    expect(kindToSingularLabel('Certificate')).toBe('Certificate');
    expect(kindToSingularLabel('ClusterIssuer')).toBe('Cluster Issuer');
  });

  it('keeps acronyms intact', () => {
    expect(kindToSingularLabel('FSCInstallation')).toBe('FSC Installation');
    expect(kindToSingularLabel('ClusterHTTPRoute')).toBe('Cluster HTTP Route');
  });

  it('returns an empty string for an empty kind', () => {
    expect(kindToSingularLabel('')).toBe('');
  });
});

describe('crdRefToLabel', () => {
  it('drops the API group and does not pluralize again', () => {
    // kindToLabel gave "certificates.cert-manager.ios" here.
    expect(crdRefToLabel('certificates.cert-manager.io')).toBe('Certificates');
    expect(crdRefToLabel('fscinstallations.openfsc.fundament.io')).toBe('Fscinstallations');
  });

  it('accepts a bare plural', () => {
    expect(crdRefToLabel('certificates')).toBe('Certificates');
  });

  it('returns an empty string for an empty reference', () => {
    expect(crdRefToLabel('')).toBe('');
  });
});

describe('labelMidSentence', () => {
  it('lowercases the words that are not acronyms', () => {
    // A plain toLowerCase() wrote "no dns endpoints found".
    expect(labelMidSentence('DNS Endpoints')).toBe('DNS endpoints');
    expect(labelMidSentence('Certificate Requests')).toBe('certificate requests');
    expect(labelMidSentence('FSC Installations')).toBe('FSC installations');
  });

  it('keeps a pluralized acronym intact', () => {
    // The plural "s" kindToLabel appends leaves the acronym an acronym; reading
    // "HTTPs" as an ordinary word wrote "cluster https".
    expect(labelMidSentence(kindToLabel('ClusterHTTP'))).toBe('cluster HTTPs');
    expect(labelMidSentence(kindToLabel('FSC'))).toBe('FSCs');
    expect(labelMidSentence(kindToLabel('ClusterHTTPRoute'))).toBe('cluster HTTP routes');
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
