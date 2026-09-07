import {
  PLATFORM_ID,
  PendingTasks,
  StateKey,
  TransferState,
  inject,
  makeStateKey,
} from '@angular/core';
import { isPlatformServer } from '@angular/common';
import { Observable, of } from 'rxjs';
import {
  DescMessage,
  DescMethodUnary,
  JsonValue,
  MessageInitShape,
  MessageShape,
  create,
  fromJson,
  toJson,
} from '@bufbuild/protobuf';
import { UnaryInterceptor } from './observable-client';

// A call is identified by the method it targets plus the request it carries.
// Proto3 JSON emits fields in schema order, so the same request always produces
// the same key on both sides of the wire.
function stateKeyFor<I extends DescMessage, O extends DescMessage>(
  method: DescMethodUnary<I, O>,
  request: MessageInitShape<I>,
): StateKey<JsonValue> | null {
  try {
    const requestJson = JSON.stringify(toJson(method.input, create(method.input, request)));
    return makeStateKey<JsonValue>(
      `connect:${method.parent.typeName}/${method.name}:${requestJson}`,
    );
  } catch {
    // A request that will not round-trip through JSON simply is not cached.
    return null;
  }
}

/**
 * Carries the results of the Connect calls a server render makes over to the
 * browser, and keeps the render waiting for them.
 *
 * Two problems, one seam. Without the first half, every call the server made
 * would be made again by the browser moments later, and the page would flicker
 * from server data to a loading state and back. Without the second half the
 * page would be blank: this app is zoneless, so the only thing that holds off
 * serialization is a registered pending task, and a bare `fetch` registers
 * nothing.
 *
 * Must be called in an injection context.
 */
export default function createTransferCacheInterceptor(): UnaryInterceptor {
  const transferState = inject(TransferState);
  const pendingTasks = inject(PendingTasks);
  const isServer = isPlatformServer(inject(PLATFORM_ID));

  return <I extends DescMessage, O extends DescMessage>(
    method: DescMethodUnary<I, O>,
    request: MessageInitShape<I>,
    call: () => Observable<MessageShape<O>>,
  ): Observable<MessageShape<O>> => {
    const key = stateKeyFor(method, request);
    if (!key) {
      return call();
    }

    if (!isServer) {
      if (!transferState.hasKey(key)) {
        return call();
      }

      const transferred = transferState.get(key, null);
      // Consume it: a repeat of the same call later in the session is a
      // deliberate refresh and should reach the network.
      transferState.remove(key);

      return transferred === null ? call() : of(fromJson(method.output, transferred));
    }

    return new Observable<MessageShape<O>>((subscriber) => {
      const taskDone = pendingTasks.add();
      let settled = false;
      const settle = () => {
        if (!settled) {
          settled = true;
          taskDone();
        }
      };

      const subscription = call().subscribe({
        next: (message) => {
          try {
            transferState.set(key, toJson(method.output, message));
          } catch {
            // Not serializable: the browser will fetch it again, which is the
            // behaviour we had before this cache existed.
          }
          subscriber.next(message);
        },
        error: (error: unknown) => {
          settle();
          subscriber.error(error);
        },
        complete: () => {
          settle();
          subscriber.complete();
        },
      });

      return () => {
        settle();
        subscription.unsubscribe();
      };
    });
  };
}
