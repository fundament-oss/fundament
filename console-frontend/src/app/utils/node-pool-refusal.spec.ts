import { Code, ConnectError } from '@connectrpc/connect';

import { GENERIC_REFUSAL, describeRefusals, refusalReason } from './node-pool-refusal';

describe('refusalReason', () => {
  it('passes a quota refusal through in the API words', () => {
    const error = new ConnectError('the organization has no quota for machine type small in region local', Code.ResourceExhausted);
    expect(refusalReason(error)).toBe('the organization has no quota for machine type small in region local');
  });

  it('keeps internal failures to a generic line', () => {
    expect(refusalReason(new ConnectError('failed to create node pool: pq: boom', Code.Internal))).toBe(GENERIC_REFUSAL);
    expect(refusalReason(new Error('network down'))).toBe(GENERIC_REFUSAL);
    expect(refusalReason(undefined)).toBe(GENERIC_REFUSAL);
  });
});

describe('describeRefusals', () => {
  it('names each pool with its own reason', () => {
    expect(
      describeRefusals([
        { name: 'workers', reason: 'over the quota of 5' },
        { name: 'spot', reason: GENERIC_REFUSAL },
      ]),
    ).toBe(`'workers': over the quota of 5; 'spot': ${GENERIC_REFUSAL}`);
  });
});
