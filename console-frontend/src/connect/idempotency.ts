import { DestroyRef, inject } from '@angular/core';
import { type CallOptions } from '@connectrpc/connect';
import { type Observable, firstValueFrom } from 'rxjs';

export const IDEMPOTENCY_STATUS = {
  PROCESSING: 'processing',
  COMPLETED: 'completed',
  FAILED: 'failed',
} as const;


export interface IdempotencyOptions {
  /** Milliseconds between polling attempts. Default: 1000 */
  pollIntervalMs?: number;
  /** Max total wait time in ms before rejecting. Default: 300_000 (5 min) */
  timeoutMs?: number;
  /** AbortSignal to cancel polling early (e.g. on component destroy). */
  signal?: AbortSignal;
}

export interface IdempotencyRef {
  /** Aborts any in-flight call and returns a fresh AbortSignal for the next one. */
  reset(): AbortSignal;
}

/**
 * Creates an idempotency controller tied to the current injection context.
 * The active AbortController is automatically aborted on component/directive destroy.
 * Must be called inside an injection context (e.g. a field initializer or constructor).
 */
export function createIdempotencyRef(): IdempotencyRef {
  const destroyRef = inject(DestroyRef);
  let controller = new AbortController();
  destroyRef.onDestroy(() => controller.abort());
  return {
    reset(): AbortSignal {
      controller.abort();
      controller = new AbortController();
      return controller.signal;
    },
  };
}

export async function withIdempotency<T>(
  call: (options: CallOptions) => Observable<T>,
  options: IdempotencyOptions = {},
): Promise<T> {
  const { pollIntervalMs = 1000, timeoutMs = 300_000, signal } = options;
  const key = crypto.randomUUID();
  const deadline = Date.now() + timeoutMs;

  const attempt = async (): Promise<{ response: T; status: string }> => {
    let idempotencyStatus: string = IDEMPOTENCY_STATUS.PROCESSING;
    const response = await firstValueFrom(
      call({
        headers: { 'Idempotency-Key': key },
        onHeader: (headers) => {
          idempotencyStatus =
            headers.get('Idempotency-Status') ?? IDEMPOTENCY_STATUS.PROCESSING;
        },
        signal,
      }),
    );
    return { response, status: idempotencyStatus };
  };

  while (true) {
    if (signal?.aborted) throw new DOMException('Idempotency polling aborted', 'AbortError');
    if (Date.now() >= deadline) throw new Error(`Idempotency timed out after ${timeoutMs}ms`);

    // Sequential polling: each attempt must complete before the next begins.
    // eslint-disable-next-line no-await-in-loop
    const { response, status } = await attempt();

    if (status === IDEMPOTENCY_STATUS.COMPLETED) return response;
    if (status === IDEMPOTENCY_STATUS.FAILED) throw new Error('Idempotency processing failed');

    // eslint-disable-next-line no-await-in-loop
    await new Promise<void>((resolve, reject) => {
      const timer = setTimeout(resolve, pollIntervalMs);
      signal?.addEventListener(
        'abort',
        () => {
          clearTimeout(timer);
          reject(new DOMException('Idempotency polling aborted', 'AbortError'));
        },
        { once: true },
      );
    });
  }
}
