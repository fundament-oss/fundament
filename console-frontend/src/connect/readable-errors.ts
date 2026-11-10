import { Code, ConnectError, type Interceptor } from '@connectrpc/connect';

/** The API, or the authorization service behind it, cannot be reached right
 *  now: a gateway 503, the outbox circuit breaker, OpenFGA down, a request that
 *  never got an answer. None of them carries text worth showing, and asking
 *  again later may work. */
export const UNAVAILABLE_MESSAGE = 'Fundament is temporarily unavailable. Try again in a moment.';

/** An internal failure, or one the browser could not classify. Their text is
 *  backend detail that is not the reader's to read; the backend logs it. */
export const INTERNAL_MESSAGE = 'Something went wrong on our side. Try again later.';

function readableText(error: ConnectError): string {
  switch (error.code) {
    case Code.Unavailable:
      return UNAVAILABLE_MESSAGE;
    case Code.Internal:
    case Code.Unknown:
      return INTERNAL_MESSAGE;
    default:
      return error.rawMessage;
  }
}

/**
 * A failed call as a page can show it: `message` is a line meant for the
 * reader, without the `[code]` prefix Connect puts in front of it, so a page
 * can write `Failed to load project: ${err.message}` as it is. The code stays,
 * for pages that branch on it, and what the server said stays in `cause`.
 *
 * A reason that is not a ConnectError yet is fetch failing outright: the
 * request never got an answer, which is what Unavailable means.
 */
export function readableConnectError(reason: unknown): ConnectError {
  const error = ConnectError.from(reason, Code.Unavailable);
  const text = readableText(error);
  const readable = new ConnectError(text, error.code, error.metadata, undefined, error);
  readable.details = error.details;
  readable.message = text;
  return readable;
}

/** Rewrites what fails while the messages of a stream are read, which is
 *  after the call itself has returned. */
async function* readableMessages<T>(
  messages: AsyncIterable<T>,
  signal: AbortSignal,
): AsyncIterable<T> {
  try {
    yield* messages;
  } catch (reason) {
    throw signal.aborted ? reason : readableConnectError(reason);
  }
}

/**
 * Makes every failed call readable once, here, rather than at each page that
 * shows one. A call that was aborted is left alone: the transport turns that
 * into Canceled itself, and nobody is shown it.
 */
export const readableErrors: Interceptor = (next) => async (req) => {
  try {
    const res = await next(req);
    return res.stream ? { ...res, message: readableMessages(res.message, req.signal) } : res;
  } catch (reason) {
    throw req.signal.aborted ? reason : readableConnectError(reason);
  }
};
