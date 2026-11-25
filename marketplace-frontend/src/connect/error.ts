import { ConnectError } from '@connectrpc/connect';

export default function connectErrorMessage(err: unknown): string {
  // rawMessage leaves out the "[code]" prefix message carries: the text is
  // for the person on the page, not for a log.
  if (err instanceof ConnectError) return err.rawMessage;
  if (err instanceof Error) return err.message;
  return 'An unexpected error occurred';
}
