import { Code } from '@connectrpc/connect';
import { failedStatus } from './metal-metrics.service';

describe('failedStatus', () => {
  it('tells an unconfigured environment, an empty store and an outage apart', () => {
    expect(failedStatus(Code.FailedPrecondition)).toBe('not-configured');
    expect(failedStatus(Code.NotFound)).toBe('no-data');
    expect(failedStatus(Code.Unavailable)).toBe('unavailable');
    expect(failedStatus(Code.Internal)).toBe('unavailable');
  });
});
