import type { Member } from '../../generated/v1/member_pb';

/**
 * Each organization member's email address by user ID. A project member is
 * always an organization member, and the project pages load the organization
 * list anyway, so this is where they find a project member's address. Unlike
 * the role lookup next to it, it keeps every row: an address is an address
 * whether or not its owner has signed in yet.
 */
export default function emailsByUserId(members: readonly Member[]): Map<string, string> {
  const emails = new Map<string, string>();
  members.forEach((member) => {
    if (member.email) emails.set(member.userId, member.email);
  });
  return emails;
}
