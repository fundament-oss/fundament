import {
  crdRefToLabel,
  formatColumnValue,
  printerColumnValue,
  printedConditionBadge,
  resolveJsonPath,
  fieldNameToLabel,
  kindToLabel,
  kindToSingularLabel,
  labelMidSentence,
  resourceListLabel,
  resourceSingularLabelMidSentence,
  statusRows,
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

describe('resourceListLabel', () => {
  it('prefers the menu label', () => {
    expect(
      resourceListLabel({ label: 'Databases' }, 'Cluster', 'clusters.postgresql.cnpg.io'),
    ).toBe('Databases');
  });

  it('falls back to the kind', () => {
    expect(resourceListLabel({}, 'DiskPool', 'diskpools.ceph.fundament.io')).toBe('Disk Pools');
  });

  it('falls back to the CRD reference without pluralizing it', () => {
    expect(resourceListLabel(undefined, undefined, 'certificates.cert-manager.io')).toBe(
      'Certificates',
    );
  });
});

describe('resourceSingularLabelMidSentence', () => {
  it('prefers the singular menu label', () => {
    expect(resourceSingularLabelMidSentence({ singularLabel: 'Database' }, 'Cluster')).toBe(
      'database',
    );
  });

  it('falls back to the kind, keeping acronyms', () => {
    expect(resourceSingularLabelMidSentence(undefined, 'DiskPool')).toBe('disk pool');
    expect(resourceSingularLabelMidSentence(undefined, 'FSCInstallation')).toBe('FSC installation');
  });
});

describe('resolveJsonPath', () => {
  const gateway = {
    metadata: { name: 'web' },
    status: {
      addresses: [
        { type: 'IPAddress', value: '194.135.48.10' },
        { type: 'IPAddress', value: '2001:db8::1' },
      ],
      conditions: [
        { type: 'Accepted', status: 'True' },
        { type: 'Programmed', status: 'False' },
      ],
    },
  };

  it('reads a field path', () => {
    expect(resolveJsonPath(gateway, '.metadata.name')).toBe('web');
  });

  it('gives every value a [*] reaches', () => {
    expect(resolveJsonPath(gateway, '.status.addresses[*].value')).toEqual([
      '194.135.48.10',
      '2001:db8::1',
    ]);
  });

  it('gives undefined for a [*] that reaches nothing', () => {
    expect(resolveJsonPath({ status: {} }, '.status.addresses[*].value')).toBeUndefined();
  });

  it('reads an index, also from the end', () => {
    expect(resolveJsonPath(gateway, '.status.addresses[0].value')).toBe('194.135.48.10');
    expect(resolveJsonPath(gateway, '.status.addresses[-1].value')).toBe('2001:db8::1');
  });

  it('reads the field of a filtered condition', () => {
    expect(resolveJsonPath(gateway, '.status.conditions[?(@.type=="Programmed")].status')).toBe(
      'False',
    );
    expect(resolveJsonPath(gateway, ".status.conditions[?(@.type == 'Accepted')].status")).toBe(
      'True',
    );
    expect(
      resolveJsonPath(gateway, '.status.conditions[?(@.type=="Ready")].status'),
    ).toBeUndefined();
  });

  it('gives undefined for a path it cannot read', () => {
    expect(resolveJsonPath(gateway, '.status.conditions[?(@.type!="Ready")]')).toBeUndefined();
  });
});

describe('formatColumnValue', () => {
  it('joins several values with commas, as kubectl does', () => {
    expect(formatColumnValue(['194.135.48.10', '2001:db8::1'], 'string')).toBe(
      '194.135.48.10, 2001:db8::1',
    );
  });
});

describe('printerColumnValue', () => {
  it('reads a top-level field, not only metadata, spec and status', () => {
    const resource = {
      apiVersion: 'gateway.networking.k8s.io/v1',
      kind: 'Gateway',
      metadata: { name: 'web', uid: 'u', creationTimestamp: '2026-11-19T10:00:00Z' },
      spec: {},
    };
    expect(printerColumnValue(resource, { name: 'Kind', jsonPath: '.kind', type: 'string' })).toBe(
      'Gateway',
    );
  });
});

describe('printedConditionBadge', () => {
  const ready = {
    name: 'Ready',
    jsonPath: '.status.conditions[?(@.type=="Ready")].status',
    type: 'string',
  };
  const phase = { name: 'Phase', jsonPath: '.status.phase', type: 'string' };

  it('names the condition after the column', () => {
    expect(printedConditionBadge(ready, 'True')).toEqual({ text: 'Ready', color: 'success' });
    expect(printedConditionBadge(ready, 'False')).toEqual({ text: 'Not ready', color: 'warning' });
  });

  it('reads Unknown as a condition only from a column on the conditions', () => {
    expect(printedConditionBadge(ready, 'Unknown')).toEqual({
      text: 'Ready unknown',
      color: 'neutral',
    });
    expect(printedConditionBadge(phase, 'Unknown')).toBeNull();
  });

  it('leaves any other value as text', () => {
    expect(printedConditionBadge(phase, 'Ready')).toBeNull();
    expect(printedConditionBadge(ready, '\u2014')).toBeNull();
  });
});

describe('statusRows', () => {
  const rows = (name: string, value: unknown) =>
    statusRows(name, value, fieldNameToLabel(name)).map(({ key, ...row }) => row);

  it('reads a scalar as one row', () => {
    expect(rows('phase', 'Ready')).toEqual([
      { type: 'value', depth: 0, label: 'Phase', value: 'Ready' },
    ]);
  });

  it('reads a Gateway address by its value', () => {
    expect(
      rows('addresses', [
        { type: 'IPAddress', value: '172.18.0.2' },
        { type: 'IPAddress', value: '172.18.0.3' },
      ]),
    ).toEqual([{ type: 'value', depth: 0, label: 'Addresses', value: '172.18.0.2, 172.18.0.3' }]);
  });

  it('lays a Gateway listener out under its name', () => {
    const accepted = { type: 'Accepted', status: 'True', reason: 'Accepted' };
    expect(
      rows('listeners', [
        {
          name: 'http',
          attachedRoutes: 2,
          supportedKinds: [
            { group: 'gateway.networking.k8s.io', kind: 'HTTPRoute' },
            { group: 'gateway.networking.k8s.io', kind: 'GRPCRoute' },
          ],
          conditions: [accepted],
        },
      ]),
    ).toEqual([
      { type: 'heading', depth: 0, label: 'Listeners' },
      { type: 'heading', depth: 1, label: 'http' },
      { type: 'value', depth: 2, label: 'Attached Routes', value: '2' },
      { type: 'value', depth: 2, label: 'Supported Kinds', value: 'HTTPRoute, GRPCRoute' },
      { type: 'conditions', depth: 2, label: 'Conditions', conditions: [accepted] },
    ]);
  });

  it('keeps every field of a small item that is not a known one-word shape', () => {
    expect(rows('ports', [{ name: 'http', port: 80 }])).toEqual([
      { type: 'heading', depth: 0, label: 'Ports' },
      { type: 'heading', depth: 1, label: 'http' },
      { type: 'value', depth: 2, label: 'Port', value: '80' },
    ]);
    expect(rows('kinds', [{ kind: 'HTTPRoute' }, { group: 'g', kind: 'GRPCRoute' }])).toEqual([
      { type: 'value', depth: 0, label: 'Kinds', value: 'HTTPRoute, GRPCRoute' },
    ]);
  });

  it('numbers items without a name, and only when there are several', () => {
    const item = (replicas: number) => ({ replicas, ready: true, zone: 'a' });
    expect(rows('pools', [item(1)])).toEqual([
      { type: 'heading', depth: 0, label: 'Pools' },
      { type: 'value', depth: 1, label: 'Replicas', value: '1' },
      { type: 'value', depth: 1, label: 'Ready', value: 'true' },
      { type: 'value', depth: 1, label: 'Zone', value: 'a' },
    ]);
    expect(rows('pools', [item(1), item(2)]).filter((row) => row.type === 'heading')).toEqual([
      { type: 'heading', depth: 0, label: 'Pools' },
      { type: 'heading', depth: 1, label: '1' },
      { type: 'heading', depth: 1, label: '2' },
    ]);
  });

  it('lays an object out under its name, a row per key', () => {
    expect(rows('certificate', { notAfter: null, serial: 'abc' })).toEqual([
      { type: 'heading', depth: 0, label: 'Certificate' },
      { type: 'value', depth: 1, label: 'Not After', value: '\u2014' },
      { type: 'value', depth: 1, label: 'Serial', value: 'abc' },
    ]);
  });

  it('shows an empty list or object as a dash', () => {
    expect(rows('addresses', [])).toEqual([
      { type: 'value', depth: 0, label: 'Addresses', value: '\u2014' },
    ]);
    expect(rows('selector', {})).toEqual([
      { type: 'value', depth: 0, label: 'Selector', value: '\u2014' },
    ]);
  });

  it('gives every row a unique key', () => {
    const keys = statusRows(
      'listeners',
      [
        { name: 'http', attachedRoutes: 0 },
        { name: 'https', attachedRoutes: 1 },
      ],
      'Listeners',
    ).map((row) => row.key);
    expect(new Set(keys).size).toBe(keys.length);
  });
});
