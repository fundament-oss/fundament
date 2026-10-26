import { Code, ConnectError } from '@connectrpc/connect';

/** A node pool the add-cluster wizard asked for and the API refused. */
export interface NodePoolNotCreated {
  name: string;
  /** The API's own words where they are meant for the reader, else a generic line. */
  reason: string;
}

/** Codes whose message is written for the user: a quota that is full, a value
 *  the catalog does not offer, a name already taken, a set read before
 *  someone else changed it. Anything else, an internal
 *  failure in particular, carries backend detail that is not theirs to read. */
const READABLE_CODES = new Set([
  Code.Aborted,
  Code.ResourceExhausted,
  Code.InvalidArgument,
  Code.FailedPrecondition,
  Code.AlreadyExists,
]);

export const GENERIC_REFUSAL = 'The platform could not create it; try again later.';

/** The API's own words for a refusal meant for the reader, if it is one. */
export function readableRefusal(error: unknown): string | undefined {
  if (error instanceof ConnectError && READABLE_CODES.has(error.code)) return error.rawMessage;
  return undefined;
}

/** The reason a request was refused, as a line to show. */
export function refusalReason(error: unknown): string {
  return readableRefusal(error) ?? GENERIC_REFUSAL;
}

/** One line for a banner: each pool by name with its reason, separated so the
 *  reader can tell which refusal belongs to which pool. */
export function describeRefusals(pools: NodePoolNotCreated[]): string {
  return pools.map((pool) => `'${pool.name}': ${pool.reason}`).join('; ');
}
