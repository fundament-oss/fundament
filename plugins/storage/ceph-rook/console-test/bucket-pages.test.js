// Outside console/, which console.go embeds and serves to the browser.

import { describe, expect, test } from 'bun:test';
import { quantityError } from '../console/_shared.js';
import { bucketNameError, requestedBucket } from '../console/bucket-pages.js';

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
