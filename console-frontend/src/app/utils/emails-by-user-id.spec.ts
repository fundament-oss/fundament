import { create } from '@bufbuild/protobuf';
import { MemberSchema } from '../../generated/v1/member_pb';
import emailsByUserId from './emails-by-user-id';

describe('emailsByUserId', () => {
  it('maps each user ID to its address, invitations included', () => {
    const emails = emailsByUserId([
      create(MemberSchema, { userId: 'u-1', email: 'alice@example.com', status: 'accepted' }),
      create(MemberSchema, { userId: 'u-2', email: 'bob@example.com', status: 'pending' }),
    ]);

    expect(emails.get('u-1')).toBe('alice@example.com');
    expect(emails.get('u-2')).toBe('bob@example.com');
  });

  it('leaves out a member without an address', () => {
    const emails = emailsByUserId([create(MemberSchema, { userId: 'u-1' })]);

    expect(emails.has('u-1')).toBe(false);
  });
});
