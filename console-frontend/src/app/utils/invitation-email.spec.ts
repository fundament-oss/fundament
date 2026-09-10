import { buildInvitationEmail } from './invitation-email';

const base = {
  email: 'ada@example.org',
  permission: 'viewer',
  organization: 'Gemeente Fundament',
  consoleUrl: 'https://console.example.org',
};

describe('buildInvitationEmail', () => {
  it('names the organization in the subject', () => {
    expect(buildInvitationEmail(base).subject).toBe(
      'Invitation to Gemeente Fundament on Fundament Console',
    );
  });

  it('reads the role the way the rest of the console labels it', () => {
    expect(buildInvitationEmail(base).body).toContain('as an organization viewer');
    expect(buildInvitationEmail({ ...base, permission: 'admin' }).body).toContain(
      'as an organization admin',
    );
  });

  it('hands back a permission it does not recognise rather than inventing one', () => {
    // organizationPermissionLabel returns the value untouched, so only the first
    // letter is lowered and nothing is lost.
    expect(buildInvitationEmail({ ...base, permission: 'auditor' }).body).toContain(
      'as an auditor',
    );
  });

  it('tells the invitee to sign in with the address the invitation was made for', () => {
    const { body } = buildInvitationEmail(base);
    expect(body).toContain('Sign in with this email address (ada@example.org)');
    expect(body).toContain('Go to https://console.example.org');
  });

  it('breaks lines with CRLF, which is what Windows mail clients need', () => {
    const { body } = buildInvitationEmail(base);
    expect(body).toContain('\r\n');
    expect(body.split('\r\n')[0]).toBe('Hi,');
    // No bare newline anywhere: one stray \n is a break Outlook drops.
    expect(body.replace(/\r\n/g, '')).not.toContain('\n');
  });

  it('carries the breaks through the mailto encoded, not stripped', () => {
    const { mailto } = buildInvitationEmail(base);
    expect(mailto).toContain('%0D%0A');
    expect(mailto.startsWith('mailto:ada%40example.org?subject=')).toBe(true);
  });

  it('encodes characters that would otherwise end the mailto early', () => {
    // & starts another mailto parameter and # starts a fragment, so an
    // organization called either would truncate the body in the mail client.
    const { mailto, body } = buildInvitationEmail({
      ...base,
      organization: 'Zorg & Welzijn #2',
    });
    expect(body).toContain('Zorg & Welzijn #2');
    expect(mailto).not.toContain('Welzijn #2');
    expect(mailto).toContain('Zorg%20%26%20Welzijn%20%232');
  });

  it('reports the invited address as the recipient, unencoded', () => {
    expect(buildInvitationEmail(base).to).toBe('ada@example.org');
  });
});
