import { AppConfiguration } from './config.service';

/**
 * Per-request context handed from the Node server to the Angular application
 * (see `server.ts`), readable through the `REQUEST_CONTEXT` injection token.
 *
 * The browser reads its runtime configuration from `/assets/config/config.json`,
 * which the server cannot do: a relative URL has no origin under Node. The
 * server reads the same file from disk once at start-up and passes the result
 * in here instead.
 */
export default interface ServerContext {
  config: AppConfiguration;
}
