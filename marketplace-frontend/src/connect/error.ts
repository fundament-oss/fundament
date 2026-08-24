import { ConnectError } from '@connectrpc/connect';

export default function connectErrorMessage(err: unknown): string {
  if (err instanceof ConnectError) return err.message;
  if (err instanceof Error) return err.message;
  return 'An unexpected error occurred';
}
