// Outside console/, which console.go embeds and serves to the browser.

import { describe, expect, test } from 'bun:test';
import { quantityError } from '../console/_shared.js';
import {
  bucketNameError,
  claimRowHtml,
  hasNoObjectStorage,
  objectStorageClassError,
  objectStorageClasses,
  requestedBucket,
  storeFieldHtml,
} from '../console/bucket-pages.js';

describe('quantityError', () => {
  test.each([['10Gi'], ['500Mi'], ['1Ti'], ['2k'], ['1E3'], ['1.5G']])('accepts %s', (q) => {
    expect(quantityError(q)).toBeNull();
  });
  test.each([
    // Uppercase K is not a Kubernetes quantity suffix.
    ['10K'],
    ['10 gigs'],
    ['Gi'],
    ['10Gib'],
  ])('rejects %s', (q) => {
    expect(quantityError(q)).not.toBeNull();
  });
});

describe('bucketNameError', () => {
  test.each([['abc'], ['my-bucket'], ['a1-2b'], ['a'.repeat(63)]])('accepts %s', (n) => {
    expect(bucketNameError(n)).toBeNull();
  });
  test.each([
    ['ab'], // below S3's 3-char floor
    ['a'.repeat(64)],
    ['Has-Upper'],
    ['-leading'],
    ['trailing-'],
    ['dots.break.tls'],
  ])('rejects %s', (n) => {
    expect(bucketNameError(n)).not.toBeNull();
  });
});

describe('requestedBucket', () => {
  test('exact name wins', () => {
    expect(requestedBucket({ bucketName: 'logs', generateBucketName: 'x' })).toBe('logs');
  });
  test('generated prefix is marked as such', () => {
    expect(requestedBucket({ generateBucketName: 'logs' })).toBe('logs-…');
  });
  test('no request renders a dash', () => {
    expect(requestedBucket({})).toBe('—');
  });
});

describe('objectStorageClasses', () => {
  const owned = (name, store, overrides = {}) => ({
    metadata: {
      name,
      ownerReferences: [{
        apiVersion: 'ceph.fundament.io/v1alpha1', kind: 'ObjectStorage', name: store, controller: true, ...overrides,
      }],
    },
    provisioner: 'rook-ceph.ceph.rook.io/bucket',
  });

  test('keeps the classes an ObjectStorage controls, sorted by store', () => {
    expect(objectStorageClasses([owned('cephobj-zeta', 'zeta'), owned('cephobj-alpha', 'alpha')])).toEqual([
      { name: 'cephobj-alpha', store: 'alpha' },
      { name: 'cephobj-zeta', store: 'zeta' },
    ]);
  });
  test('drops block and file classes', () => {
    expect(objectStorageClasses([owned('ceph-fast', 'fast', { kind: 'BlockStorage' })])).toEqual([]);
  });
  test('drops a class merely named like one', () => {
    expect(objectStorageClasses([{ metadata: { name: 'cephobj-x' }, provisioner: 'rook-ceph.ceph.rook.io/bucket' }]))
      .toEqual([]);
  });
  test('drops a non-controller reference and another group', () => {
    expect(objectStorageClasses([
      owned('a', 'a', { controller: false }),
      owned('b', 'b', { apiVersion: 'example.com/v1' }),
    ])).toEqual([]);
  });
});

describe('claimRowHtml', () => {
  const claim = {
    metadata: { name: 'logs', namespace: 'tnt-acme--web' },
    spec: { generateBucketName: 'logs', storageClassName: 'cephobj-s3' },
    status: { phase: 'Bound' },
  };

  test('renders the section columns with the raw namespace', () => {
    const html = claimRowHtml(claim);
    expect(html).toContain('data-name="logs"');
    expect(html).toContain('data-namespace="tnt-acme--web"');
    expect(html).toContain('<td>tnt-acme--web</td>');
    expect(html).toContain('<td>Bound</td>');
    expect(html).toContain('<td>logs-…</td>');
    expect(html).not.toContain('cephobj-s3');
  });
  test('shows the namespace display name and the storage class on request', () => {
    const html = claimRowHtml(claim, { displayNames: { 'tnt-acme--web': 'web' }, storageClass: true });
    expect(html).toContain('data-namespace="tnt-acme--web"');
    expect(html).toContain('<td>web</td>');
    expect(html).toContain('<td>cephobj-s3</td>');
  });
  test('escapes claim fields', () => {
    expect(claimRowHtml({ metadata: { name: '<x>' } })).toContain('&lt;x&gt;');
  });
});

describe('objectStorageClassError', () => {
  test.each([['cephobj-s3'], ['cephobj-a'], ['cephobj-my-store']])('accepts %s', (v) => {
    expect(objectStorageClassError(v)).toBeNull();
  });
  test.each([
    [''],
    ['cephobj-'],
    ['ceph-fast'], // a BlockStorage class
    ['cephfs-shared'], // a FileStorage class
    ['standard'],
    ['cephobj-Upper'],
  ])('rejects %s', (v) => {
    expect(objectStorageClassError(v)).not.toBeNull();
  });
});

describe('hasNoObjectStorage', () => {
  test('an empty list is the empty state', () => {
    expect(hasNoObjectStorage([])).toBe(true);
  });
  test('a forbidden list (null) falls back to the form', () => {
    expect(hasNoObjectStorage(null)).toBe(false);
  });
  test('any store shows the form', () => {
    expect(hasNoObjectStorage([{ name: 'cephobj-s3', store: 's3' }])).toBe(false);
  });
});

describe('storeFieldHtml', () => {
  test('unknown classes take the name as text, with the convention', () => {
    const html = storeFieldHtml(null);
    expect(html).toContain('<nldd-text-field');
    expect(html).toContain('cephobj-&lt;store name&gt;');
    expect(html).not.toContain('<select');
  });
  test('one store is preselected', () => {
    const html = storeFieldHtml([{ name: 'cephobj-s3', store: 's3' }]);
    expect(html).toContain('<option value="cephobj-s3" selected>s3</option>');
    expect(html).not.toContain('disabled');
  });
  test('several stores start on a disabled placeholder', () => {
    const html = storeFieldHtml([{ name: 'cephobj-a', store: 'a' }, { name: 'cephobj-b', store: 'b' }]);
    expect(html).toContain('<option value="" selected disabled>Choose an object storage</option>');
    expect(html).toContain('<option value="cephobj-a">a</option>');
    expect(html).toContain('<option value="cephobj-b">b</option>');
  });
});
