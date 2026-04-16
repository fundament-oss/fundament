import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { Observable } from 'rxjs';
import { type CallOptions } from '@connectrpc/connect';
import { withIdempotency } from './idempotency';

interface FakeResponse {
  clusterId: string;
}

function makeCall(statuses: string[], response: FakeResponse = { clusterId: 'abc' }) {
  let callCount = 0;
  return vi.fn((options: CallOptions) => {
    const status = statuses[callCount] ?? 'completed';
    callCount += 1;
    return new Observable<FakeResponse>((sub) => {
      options.onHeader?.(new Headers({ 'Idempotency-Status': status }));
      sub.next(response);
      sub.complete();
    });
  });
}

describe('withIdempotency', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it('resolves immediately when status is completed on first call', async () => {
    const call = makeCall(['completed']);
    const result = await withIdempotency(call);
    expect(result).toEqual({ clusterId: 'abc' });
    expect(call).toHaveBeenCalledTimes(1);
  });

  it('polls until completed', async () => {
    const call = makeCall(['processing', 'processing', 'completed']);
    const promise = withIdempotency(call, { pollIntervalMs: 100 });
    await vi.advanceTimersByTimeAsync(300);
    expect(await promise).toEqual({ clusterId: 'abc' });
    expect(call).toHaveBeenCalledTimes(3);
  });

  it('rejects when status is failed', async () => {
    const call = makeCall(['failed']);
    await expect(withIdempotency(call)).rejects.toThrow('Idempotency processing failed');
    expect(call).toHaveBeenCalledTimes(1);
  });

  it('rejects on timeout', async () => {
    // pollIntervalMs: 100, timeoutMs: 150 — deadline is hit after the first delay,
    // before a second attempt would return 'completed'.
    const call = makeCall(['processing', 'processing', 'processing']);
    const promise = withIdempotency(call, { pollIntervalMs: 100, timeoutMs: 150 });
    const assertion = expect(promise).rejects.toThrow('timed out');
    await vi.advanceTimersByTimeAsync(300);
    await assertion;
  });

  it('propagates network errors out of the polling loop', async () => {
    const networkError = new Error('Network failure');
    const call = vi.fn(
      () =>
        new Observable<FakeResponse>((sub) => {
          sub.error(networkError);
        }),
    );
    await expect(withIdempotency(call)).rejects.toThrow('Network failure');
    expect(call).toHaveBeenCalledTimes(1);
  });

  it('rejects when AbortSignal is triggered during polling delay', async () => {
    const controller = new AbortController();
    const call = makeCall(['processing']);
    const promise = withIdempotency(call, { signal: controller.signal, pollIntervalMs: 1000 });
    // Allow the first attempt to complete (status: processing), then abort during the delay.
    // Attach .catch before aborting so the rejection is handled synchronously.
    await vi.advanceTimersByTimeAsync(0);
    const caught = promise.catch((err: unknown) => err);
    controller.abort();
    await vi.advanceTimersByTimeAsync(0);
    expect(await caught).toMatchObject({ name: 'AbortError' });
  });

  it('rejects when AbortSignal is already aborted before the loop starts', async () => {
    const controller = new AbortController();
    controller.abort();
    const call = makeCall(['processing']);
    await expect(withIdempotency(call, { signal: controller.signal })).rejects.toMatchObject({
      name: 'AbortError',
    });
    expect(call).not.toHaveBeenCalled();
  });

  it('sends the same idempotency key on every retry', async () => {
    const keys: string[] = [];
    const call = vi.fn((options: CallOptions) => {
      const headerMap = options.headers as Record<string, string>;
      keys.push(headerMap['Idempotency-Key']);
      const status = keys.length < 3 ? 'processing' : 'completed';
      return new Observable<FakeResponse>((sub) => {
        options.onHeader?.(new Headers({ 'Idempotency-Status': status }));
        sub.next({ clusterId: 'abc' });
        sub.complete();
      });
    });
    const promise = withIdempotency(call, { pollIntervalMs: 100 });
    await vi.advanceTimersByTimeAsync(300);
    await promise;
    expect(new Set(keys).size).toBe(1);
    expect(keys).toHaveLength(3);
  });
});
