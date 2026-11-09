/** A node pool as a form asks for it. */
export interface DesiredNodePool {
  name: string;
  machineType: string;
  autoscaleMin: number;
  autoscaleMax: number;
}

/** A node pool as the API lists it. */
export interface LiveNodePool {
  name: string;
  machineType: string;
  minNodes: number;
  maxNodes: number;
}

/** Whether the live pools are exactly the desired ones: a save whose response
 *  was lost has still been applied when they are. */
export function nodePoolsMatch(live: LiveNodePool[], desired: DesiredNodePool[]): boolean {
  const key = (name: string, machineType: string, min: number, max: number) =>
    `${name}|${machineType}|${min}|${max}`;
  const liveKeys = new Set(
    live.map((pool) => key(pool.name, pool.machineType, pool.minNodes, pool.maxNodes)),
  );
  return (
    live.length === desired.length &&
    liveKeys.size === live.length &&
    desired.every((pool) =>
      liveKeys.has(key(pool.name, pool.machineType, pool.autoscaleMin, pool.autoscaleMax)),
    )
  );
}
