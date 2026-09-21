import { describe, expect, it } from 'vitest';
import { describeResource, grantsEverything, groupPermissions } from './permissions';

describe('describeResource', () => {
  it.each([
    ['secrets', 'Secrets'],
    ['validatingwebhookconfigurations', 'Validating webhooks'],
    ['clusterissuers', 'Cluster issuers'],
    ['certificaterequests', 'Certificate requests'],
    ['grpcroutes', 'gRPC routes'],
    ['dnsendpoints', 'DNS endpoints'],
    ['cephblockpools', 'Ceph block pools'],
    ['networkpolicies', 'Network policies'],
    ['gatewayclasses', 'Gateway classes'],
    ['backendtlspolicies', 'Backend TLS policies'],
    ['gateways/status', 'Status of gateways'],
    ['httproutes/status', 'Status of HTTP routes'],
    ['frobnicators', 'Frobnicators'],
    ['*', 'All resources'],
  ])('labels %s as %s', (name, label) => {
    expect(describeResource(name).label).toBe(label);
  });

  it('describes built-in resources only', () => {
    expect(describeResource('secrets').description).not.toBe('');
    expect(describeResource('clusterissuers').description).toBe('');
  });
});

describe('groupPermissions', () => {
  it('splits rules into resources grouped by access, most permissive first', () => {
    const groups = groupPermissions([
      { resource: 'orders, challenges', access: 'Read' },
      { resource: 'certificates, issuers', access: 'Read and write' },
    ]);

    expect(groups.map((group) => group.heading)).toEqual(['Can view and change', 'Can view']);
    expect(groups[0].resources.map((resource) => resource.label)).toEqual([
      'Certificates',
      'Issuers',
    ]);
    expect(groups[1].resources.map((resource) => resource.name)).toEqual(['challenges', 'orders']);
  });

  it('lists a resource once, under its most permissive access', () => {
    const groups = groupPermissions([
      { resource: 'secrets, pods', access: 'Read' },
      { resource: 'secrets', access: 'Read and write' },
    ]);

    expect(groups).toHaveLength(2);
    expect(groups[0].resources.map((resource) => resource.name)).toEqual(['secrets']);
    expect(groups[1].resources.map((resource) => resource.name)).toEqual(['pods']);
  });

  it('keeps unknown access levels after the known ones', () => {
    const groups = groupPermissions([
      { resource: 'pods', access: 'Something else' },
      { resource: 'secrets', access: 'Read' },
    ]);

    expect(groups.map((group) => group.heading)).toEqual(['Can view', 'Something else']);
  });

  it('returns nothing for no permissions', () => {
    expect(groupPermissions([])).toEqual([]);
  });
});

describe('grantsEverything', () => {
  it('detects a wildcard resource', () => {
    expect(grantsEverything([{ resource: 'pods, *', access: 'Read and write' }])).toBe(true);
    expect(grantsEverything([{ resource: 'pods', access: 'Read and write' }])).toBe(false);
  });
});
