import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { Action } from '../../generated/v1/actions_pb';
import { PermissionDeniedDetailSchema, RequiredRole } from '../../generated/v1/authz_pb';
import permissionDeniedMessage from './permission-denied';
import sentences from '../../../../common/authz/denial/testdata/sentences.json';
import noDetailSentences from '../../../../common/authz/denial/testdata/no-detail-sentences.json';

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

  it('has no-detail fixtures to check', () => {
    expect(noDetailSentences.length).toBeGreaterThan(0);
  });

  it.each(noDetailSentences)(
    'matches the shared sentence for a denial without detail: "$message"',
    ({ message, sentence }) => {
      expect(permissionDeniedMessage(new ConnectError(message, Code.PermissionDenied))).toBe(
        sentence,
      );
    },
  );

  it("shows the general sentence for connect-es's own text on a gateway 403", () => {
    expect(permissionDeniedMessage(new ConnectError('HTTP 403', Code.PermissionDenied))).toBe(
      'You do not have permission to do this.',
    );
  });

  it('uses the raw message, without a code prefix', () => {
    const err = new ConnectError('some server words', Code.PermissionDenied);
    expect(err.message).toContain('[permission_denied]');
    expect(permissionDeniedMessage(err)).toBe('some server words');
  });

  it('prefers the detail over the message', () => {
    expect(
      permissionDeniedMessage(denial(Action.CLUSTER_CAN_DELETE, RequiredRole.ORGANIZATION_ADMIN)),
    ).toBe('Only organization admins can delete clusters');
  });

  it('returns null for other errors', () => {
    expect(permissionDeniedMessage(new ConnectError('boom', Code.Internal))).toBeNull();
    expect(permissionDeniedMessage(new Error('boom'))).toBeNull();
    expect(permissionDeniedMessage(undefined)).toBeNull();
  });
});
