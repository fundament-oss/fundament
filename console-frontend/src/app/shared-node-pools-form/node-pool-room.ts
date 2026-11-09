import { NodeQuotas } from '../node-quota.service';

/** The part of a pool that counts against a node quota. */
export interface PoolDraft {
  name: string;
  machineType: string;
  autoscaleMax: number;
}

/** How much of its machine type's node quota one pool in the form may take. */
export interface PoolRoom {
  machineType: string;
  /** The organization's quota for the machine type; 0 when none is set, which
   *  allows no pool of that type. */
  quota: number;
  /** The highest maximum this pool can have, with every other pool, in this
   *  form and in the organization's other clusters, as it is now. Can be
   *  below zero when the others already take more than the quota. */
  limit: number;
  /** Whether saving would raise what the pool counts against the quota: a new
   *  pool, or a raised maximum. The API checks only that, so a pool that keeps
   *  or lowers its maximum is allowed under a quota it is already over. */
  grows: boolean;
}

const sumOfMaxima = (pools: readonly PoolDraft[], machineType: string): number =>
  pools
    .filter((pool) => pool.machineType === machineType)
    .reduce((sum, pool) => sum + (Number(pool.autoscaleMax) || 0), 0);

/**
 * Nodes of the machine type that the organization uses outside this form: in
 * use, less what the pools the form was loaded with count. Those are in "in
 * use" already, and the form holds what each of them will be after saving,
 * deleted ones by their absence.
 */
export function nodesElsewhere(
  machineType: string,
  originals: ReadonlyMap<string, PoolDraft>,
  quotas: NodeQuotas,
): number {
  const inUse = quotas.get(machineType)?.inUse ?? 0;
  return Math.max(0, inUse - sumOfMaxima([...originals.values()], machineType));
}

/**
 * The room for `pools[index]`. Pools in the form with the same machine type
 * share the room, so each one's limit is what all the others leave over.
 *
 * Saving keeps an existing pool by its name, so a pool counts as existing when
 * a pool with its name was loaded into the form.
 */
export function poolRoom(
  index: number,
  pools: readonly PoolDraft[],
  originals: ReadonlyMap<string, PoolDraft>,
  quotas: NodeQuotas,
): PoolRoom {
  const pool = pools[index];
  const { machineType } = pool;
  const quota = quotas.get(machineType)?.quota ?? 0;
  const others = sumOfMaxima(
    pools.filter((_, i) => i !== index),
    machineType,
  );
  const original = originals.get(pool.name);
  return {
    machineType,
    quota,
    limit: quota - nodesElsewhere(machineType, originals, quotas) - others,
    grows:
      !original ||
      original.machineType !== machineType ||
      Number(pool.autoscaleMax) > original.autoscaleMax,
  };
}

/** What a new pool of the machine type could have, next to the pools in the form. */
export function roomForNewPool(
  machineType: string,
  pools: readonly PoolDraft[],
  originals: ReadonlyMap<string, PoolDraft>,
  quotas: NodeQuotas,
): number {
  const quota = quotas.get(machineType)?.quota ?? 0;
  return quota - nodesElsewhere(machineType, originals, quotas) - sumOfMaxima(pools, machineType);
}
