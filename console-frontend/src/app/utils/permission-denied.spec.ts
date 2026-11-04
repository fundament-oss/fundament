import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { Action, PermissionDeniedDetailSchema, RequiredRole } from '../../generated/v1/authz_pb';
import permissionDeniedMessage from './permission-denied';
import sentences from '../../../../common/authz/denial/testdata/sentences.json';

const NO_DETAIL = 'You do not have permission to do this.';

function denial(action: Action, requiredRole: RequiredRole): ConnectError {
  return new ConnectError('denied', Code.PermissionDenied, undefined, [
    {
      desc: PermissionDeniedDetailSchema,
      value: create(PermissionDeniedDetailSchema, { action, requiredRole }),
    },
  ]);
}

describe('permissionDeniedMessage', () => {
  it('has sentence fixtures to check', () => {
    expect(sentences.length).toBeGreaterThan(0);
  });

  it.each(sentences)(
    'matches the shared sentence for $action / $role',
    ({ action, role, sentence }) => {
      const a = Action[action.replace(/^ACTION_/, '') as keyof typeof Action];
      const r = RequiredRole[role.replace(/^REQUIRED_ROLE_/, '') as keyof typeof RequiredRole];
      expect(permissionDeniedMessage(denial(a, r))).toBe(sentence);
    },
  );

  it('falls back for an unknown action', () => {
    expect(permissionDeniedMessage(denial(999 as Action, RequiredRole.ORGANIZATION_ADMIN))).toBe(
      'Only organization admins can do this',
    );
  });

  it('falls back when a denial has no detail', () => {
    expect(permissionDeniedMessage(new ConnectError('nope', Code.PermissionDenied))).toBe(
      NO_DETAIL,
    );
  });

  it('returns null for other errors', () => {
    expect(permissionDeniedMessage(new ConnectError('boom', Code.Internal))).toBeNull();
    expect(permissionDeniedMessage(new Error('boom'))).toBeNull();
    expect(permissionDeniedMessage(undefined)).toBeNull();
  });
});
