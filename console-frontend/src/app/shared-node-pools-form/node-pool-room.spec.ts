import { NodeQuotas } from '../node-quota.service';
import { PoolDraft, poolRoom, roomForNewPool } from './node-pool-room';

// The quota counts the maxima of every pool of a machine type in every cluster
// of the organization. The form holds some of those pools, as they will be
// after saving, so what it may use is the quota less what the others take.
describe('node pool room', () => {
  const quotas: NodeQuotas = new Map([
    ['local-medium', { quota: 20, inUse: 8 }],
    ['local-small', { quota: 0, inUse: 0 }],
  ]);
  const pool = (name: string, autoscaleMax: number, machineType = 'local-medium'): PoolDraft => ({
    name,
    machineType,
    autoscaleMax,
  });

  it('leaves a new pool the quota less what is in use', () => {
    const room = poolRoom(0, [pool('new', 3)], new Map(), quotas);
    expect(room.limit).toBe(12);
    expect(room.grows).toBe(true);
  });

  it('adds back what an existing pool already counts', () => {
    const originals = new Map([['workers', pool('workers', 5)]]);
    const room = poolRoom(0, [pool('workers', 5)], originals, quotas);
    expect(room.limit).toBe(17);
    expect(room.grows).toBe(false);
  });

  it('gives a second pool of the same type what the first leaves over', () => {
    const pools = [pool('a', 4), pool('b', 3)];
    expect(poolRoom(0, pools, new Map(), quotas).limit).toBe(9);
    expect(poolRoom(1, pools, new Map(), quotas).limit).toBe(8);
  });

  it('does not share room with pools of another type', () => {
    const pools = [pool('a', 4), pool('b', 3, 'local-small')];
    expect(poolRoom(0, pools, new Map(), quotas).limit).toBe(12);
  });

  it('frees the room of a pool removed from the form', () => {
    const originals = new Map([
      ['workers', pool('workers', 5)],
      ['gone', pool('gone', 3)],
    ]);
    expect(poolRoom(0, [pool('workers', 5)], originals, quotas).limit).toBe(20);
  });

  it('counts only a raised maximum as growth', () => {
    const originals = new Map([['workers', pool('workers', 5)]]);
    expect(poolRoom(0, [pool('workers', 4)], originals, quotas).grows).toBe(false);
    expect(poolRoom(0, [pool('workers', 6)], originals, quotas).grows).toBe(true);
  });

  it('treats a renamed pool as new, because saving creates it', () => {
    const originals = new Map([['workers', pool('workers', 5)]]);
    expect(poolRoom(0, [pool('renamed', 5)], originals, quotas).grows).toBe(true);
  });

  it('reports no quota for a machine type the quotas do not list', () => {
    expect(poolRoom(0, [pool('a', 1, 'local-large')], new Map(), quotas).quota).toBe(0);
  });

  it('gives a new pool the room the pools in the form leave over', () => {
    expect(roomForNewPool('local-medium', [pool('a', 10)], new Map(), quotas)).toBe(2);
    expect(roomForNewPool('local-small', [], new Map(), quotas)).toBe(0);
  });
});
