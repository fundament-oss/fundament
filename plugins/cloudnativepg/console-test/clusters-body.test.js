// Outside console/, which console.go embeds and serves to the browser.

import { describe, expect, test } from 'bun:test';
import { buildCluster, storageSizeError } from '../console/clusters-body.js';
import { humanizeQuantity } from '../console/quantity.js';

describe('storageSizeError', () => {
  test.each([
    '1Gi',
    '5Gi',
    '1.5Gi',
    '10G', // 9.3 GiB
    '1024Mi',
    '+2Gi',
    '1Ti',
    '1099511627776', // 1Ti in bytes
    '2e9',
  ])('accepts %p', (size) => {
    expect(storageSizeError(size)).toBeNull();
  });

  test.each([
    ['0Gi', 'Use at least 1Gi.'],
    ['01Gi', null], // a leading zero is still a Quantity of 1Gi
    ['1G', 'Use at least 1Gi.'], // 0.93 GiB
    ['1023Mi', 'Use at least 1Gi.'],
    ['-1Gi', 'Use at least 1Gi.'],
    ['1025Gi', 'Use at most 1Ti.'],
    // Accepted by Kubernetes, but no provisioner makes it.
    ['99999999999999999999Gi', 'Use at most 1Ti.'],
    ['1Pi', 'Use at most 1Ti.'],
  ])('%p -> %p', (size, want) => {
    expect(storageSizeError(size)).toBe(want);
  });

  // Quantity grammar, but beyond a number: 0 * 10 ** 999 is NaN, which
  // would pass both range checks.
  test.each(['', '10 Gi', '10GB', '1K', 'Gi', 'ten', '0e999', '1e999', '-0e999'])('rejects %p as no quantity', (size) => {
    expect(storageSizeError(size)).toBe('Enter a size like 10Gi, 500Mi or 20G.');
  });
});

describe('humanizeQuantity', () => {
  test.each([
    ['5Gi', '5 GiB'],
    ['1.5G', '1.3 GiB'],
    ['1Ti', '1 TiB'],
    ['garbage', 'garbage'],
    [undefined, 'Unknown'],
  ])('%p -> %p', (quantity, want) => {
    expect(humanizeQuantity(quantity)).toBe(want);
  });
});

describe('buildCluster', () => {
  const base = { name: 'orders', namespace: 'team-a', major: 17, size: '5Gi' };

  test('refers to a major in the image catalog', () => {
    expect(buildCluster(base).spec.imageCatalogRef).toEqual({
      apiGroup: 'postgresql.cnpg.io',
      kind: 'ClusterImageCatalog',
      name: 'fundament-postgresql',
      major: 17,
    });
    expect(buildCluster(base).spec.imageName).toBeUndefined();
  });

  // A single instance's PDB would block every node drain.
  test('has no PodDisruptionBudget', () => {
    expect(buildCluster(base).spec.enablePDB).toBe(false);
  });

  test('names a StorageClass only when one is chosen', () => {
    expect(buildCluster(base).spec.storage).toEqual({ size: '5Gi' });
    expect(buildCluster({ ...base, storageClass: 'ceph-fast' }).spec.storage).toEqual({
      size: '5Gi',
      storageClass: 'ceph-fast',
    });
  });
});
