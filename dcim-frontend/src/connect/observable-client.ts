// Based on: https://github.com/connectrpc/examples-es/blob/main/angular/src/connect/observable-client.ts

import { makeAnyClient, CallOptions, Transport } from '@connectrpc/connect';
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

function createUnaryFn<I extends DescMessage, O extends DescMessage>(
  transport: Transport,
  method: DescMethodUnary<I, O>,
): UnaryFn<I, O> {
  return function unary(requestMessage, options) {
    return new Observable<MessageShape<O>>((subscriber) => {
      transport
        .unary(method, options?.signal, options?.timeoutMs, options?.headers, requestMessage)
        .then(
          (response) => {
            options?.onHeader?.(response.header);
            subscriber.next(response.message);
            options?.onTrailer?.(response.trailer);
          },
          (err) => {
            subscriber.error(err);
          },
        )
        .finally(() => {
          subscriber.complete();
        });
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
      transport
        .stream<I, O>(
          method,
          options?.signal,
          options?.timeoutMs,
          options?.headers,
          createAsyncIterable([input]),
        )
        .then(
          async (streamResponse) => {
            options?.onHeader?.(streamResponse.header);
            const iterator = streamResponse.message[Symbol.asyncIterator]();
            const drain = async (): Promise<void> => {
              const result = await iterator.next();
              if (result.done) return;
              subscriber.next(result.value);
              await drain();
            };
            await drain();
            options?.onTrailer?.(streamResponse.trailer);
          },
          (err) => {
            subscriber.error(err);
          },
        )
        .finally(() => {
          subscriber.complete();
        });
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
