// Outside console/, which console.go embeds and serves to the browser.

import { describe, expect, test } from 'bun:test';
import { freeSpace, infoButtonHtml, latestRead, selectedDiskCount } from '../console/_shared.js';
import { BLOCKSTORAGE, copiesOnSeparateNodes, emptySupportingText } from '../console/consumer-pages.js';

describe('selectedDiskCount', () => {
  test.each([
    // Not reconciled yet: nothing is counted, which is not zero.
    [undefined, undefined],
    [{}, undefined],
    // omitempty drops a zero count, so a reconciled pool without the field has none.
    [{ phase: 'Degraded' }, 0],
    [{ phase: 'Ready', selectedDiskCount: 2 }, 2],
  ])('%p -> %p', (status, want) => {
    expect(selectedDiskCount(status)).toBe(want);
  });
});

describe('latestRead', () => {
  test('only the newest read is current', () => {
    const nextRead = latestRead();
    const first = nextRead();
    expect(first()).toBe(true);
    const second = nextRead();
    expect(first()).toBe(false);
    expect(second()).toBe(true);
  });

  test('each page keeps its own count', () => {
    const a = latestRead();
    const b = latestRead();
    const readA = a();
    b();
    expect(readA()).toBe(true);
  });
});

describe('freeSpace', () => {
  const cap = { bytesAvailable: 3 * 2 ** 40, bytesTotal: 6 * 2 ** 40 };

  test('divides by the replica count', () => {
    expect(freeSpace(cap, 3)).toEqual(['Free space (approx.)', '1 TiB of 2 TiB']);
  });
  test('stays raw without a replica count', () => {
    expect(freeSpace(cap, null)).toEqual(['Free in cluster (raw)', '3 TiB of 6 TiB']);
  });
  test('no capacity, no row', () => {
    expect(freeSpace(null, 3)).toBeNull();
  });
});

describe('copiesOnSeparateNodes', () => {
  test.each([
    ['host', 'Yes'],
    ['osd', 'No'],
    [undefined, '—'],
    ['rack', 'rack'],
  ])('%p -> %p', (domain, want) => {
    expect(copiesOnSeparateNodes(domain)).toBe(want);
  });
});

describe('infoButtonHtml', () => {
  test('one paragraph per text, escaped, in a popover the button opens', () => {
    const html = infoButtonHtml('About <x>', ['First.', 'A & B.']);
    expect(html).toContain('text="About &lt;x&gt;"');
    expect(html).toContain('<nldd-popover slot="popup" role="region" accessible-label="About &lt;x&gt;">');
    expect(html).toContain('<p>First.</p>');
    expect(html).toContain('<p>A &amp; B.</p>');
  });
});

describe('emptySupportingText', () => {
  const needsPool = 'Create a disk pool with disks first.';
  const create = 'Use New block storage… to create one.';
  test.each([
    ['no pools', [], needsPool],
    ['a pool without disks', [{ status: { phase: 'Degraded' } }], needsPool],
    // Not counted is not zero: the operator may still find disks for it.
    ['a pool not counted yet', [{ status: {} }], create],
    ['a pool not counted yet beside one without disks', [{ status: {} }, { status: { phase: 'Degraded' } }], create],
    ['a pool with disks', [{ status: { phase: 'Ready', selectedDiskCount: 2 } }], create],
    ['one of two pools with disks', [{ status: { phase: 'Degraded' } }, { status: { phase: 'Ready', selectedDiskCount: 1 } }], create],
    // The pools could not be read: ask for the storage, not for a pool that may exist.
    ['pools unknown', undefined, create],
  ])('%s', (_name, pools, want) => {
    expect(emptySupportingText(BLOCKSTORAGE, pools)).toBe(want);
  });
});
