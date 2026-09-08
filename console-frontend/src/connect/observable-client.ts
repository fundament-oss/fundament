// Based on: https://github.com/connectrpc/examples-es/blob/main/angular/src/connect/observable-client.ts

import { Code, ConnectError, makeAnyClient, CallOptions, Transport } from '@connectrpc/connect';
import { createAsyncIterable } from '@connectrpc/connect/protocol';
import {
  DescService,
  DescMessage,
  MessageInitShape,
  MessageShape,
  DescMethodStreaming,
  DescMethodUnary,
  DescMethodServerStreaming,
} from '@bufbuild/protobuf';
import { Observable } from 'rxjs';

export type ObservableClient<T extends DescService> = {
  [P in keyof T['method']]: T['method'][P] extends DescMethodUnary<infer I, infer O>
    ? UnaryFn<I, O>
    : T['method'][P] extends DescMethodServerStreaming<infer I, infer O>
      ? ServerStreamingFn<I, O>
      : never;
};

type UnaryFn<I extends DescMessage, O extends DescMessage> = (
  request: MessageInitShape<I>,
  options?: CallOptions,
) => Observable<MessageShape<O>>;

/**
 * Cancellation raised by our own teardown, which the caller never asked to see.
 * Connect surfaces an aborted call as a ConnectError with Code.Canceled rather
 * than a DOMException.
 */
function isCanceled(err: unknown): boolean {
  return err instanceof ConnectError && err.code === Code.Canceled;
}

/**
 * An AbortController for one call, chained to any caller-supplied signal.
 *
 * The transport passes a signal straight to fetch but registers no teardown of
 * its own, and RxJS never calls `return()` on an async iterator it stops
 * walking. Without aborting on unsubscribe the request keeps draining and the
 * server keeps producing for a subscriber that has gone away.
 */
function callController(upstream: AbortSignal | undefined): AbortController {
  const controller = new AbortController();
  if (upstream) {
    if (upstream.aborted) {
      controller.abort(upstream.reason);
    } else {
      upstream.addEventListener('abort', () => controller.abort(upstream.reason), { once: true });
    }
  }
  return controller;
}

function createUnaryFn<I extends DescMessage, O extends DescMessage>(
  transport: Transport,
  method: DescMethodUnary<I, O>,
): UnaryFn<I, O> {
  return function unary(requestMessage, options) {
    return new Observable<MessageShape<O>>((subscriber) => {
      const controller = callController(options?.signal);

      transport
        .unary(method, controller.signal, options?.timeoutMs, options?.headers, requestMessage)
        .then(
          (response) => {
            options?.onHeader?.(response.header);
            subscriber.next(response.message);
            options?.onTrailer?.(response.trailer);
            subscriber.complete();
          },
          (err: unknown) => {
            if (controller.signal.aborted && isCanceled(err)) {
              subscriber.complete();
              return;
            }
            subscriber.error(err);
          },
        );

      return () => controller.abort();
    });
  };
}

type ServerStreamingFn<I extends DescMessage, O extends DescMessage> = (
  request: MessageInitShape<I>,
  options?: CallOptions,
) => Observable<MessageShape<O>>;

export function createServerStreamingFn<I extends DescMessage, O extends DescMessage>(
  transport: Transport,
  method: DescMethodServerStreaming<I, O>,
): ServerStreamingFn<I, O> {
  return function serverStreaming(input, options) {
    return new Observable<MessageShape<O>>((subscriber) => {
      const controller = callController(options?.signal);

      const run = async (): Promise<void> => {
        const streamResponse = await transport.stream<I, O>(
          method,
          controller.signal,
          options?.timeoutMs,
          options?.headers,
          createAsyncIterable([input]),
        );
        options?.onHeader?.(streamResponse.header);
        // A for-await loop, not awaited recursion: recursion suspends the caller
        // on every message, so one frame per streamed message — each retaining
        // its message — stays alive until the stream ends. A log tail emits per
        // line, which turns that into unbounded retention.
        // eslint-disable-next-line no-restricted-syntax -- an async iterable has no array form to iterate.
        for await (const message of streamResponse.message) {
          if (subscriber.closed) return;
          subscriber.next(message);
        }
        options?.onTrailer?.(streamResponse.trailer);
      };

      // A rejection inside the drain must reach subscriber.error. Handling it in
      // the second argument of .then() would only cover stream setup, letting a
      // mid-stream failure arrive as a clean completion.
      run().then(
        () => subscriber.complete(),
        (err: unknown) => {
          if (controller.signal.aborted && isCanceled(err)) {
            subscriber.complete();
            return;
          }
          subscriber.error(err);
        },
      );

      return () => controller.abort();
    });
  };
}

export function createObservableClient<T extends DescService>(service: T, transport: Transport) {
  return makeAnyClient(service, (method: DescMethodUnary | DescMethodStreaming) => {
    switch (method.methodKind) {
      case 'unary':
        return createUnaryFn(transport, method);
      case 'server_streaming':
        return createServerStreamingFn(transport, method);
      default:
        return null;
    }
  }) as ObservableClient<T>;
}
