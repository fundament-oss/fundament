import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';
import OrgPickerComponent from './org-picker.component';
import { OrganizationSchema } from '../../generated/v1/organization_pb';
import { InvitationSchema } from '../../generated/v1/invite_pb';

const ORGANIZATION = create(OrganizationSchema, { id: 'org-1', name: 'acme-corp', alias: 'Acme' });

const INVITATION = create(InvitationSchema, {
  id: 'inv-1',
  organizationId: 'org-2',
  organizationAlias: 'Globex',
});

function build(organizations = [ORGANIZATION], invitations: (typeof INVITATION)[] = []) {
  const fixture = TestBed.createComponent(OrgPickerComponent);
  fixture.componentRef.setInput('organizations', organizations);
  fixture.componentRef.setInput('invitations', invitations);
  fixture.componentRef.setInput('userName', 'Alice');
  fixture.componentRef.setInput('userId', '019b4000-1000-7000-8000-000000000001');
  fixture.detectChanges();
  return fixture;
}

function heading(fixture: ReturnType<typeof build>): string {
  return (fixture.nativeElement as HTMLElement).querySelector('h1')?.textContent?.trim() ?? '';
}

describe('OrgPickerComponent', () => {
  it('offers the organizations someone is in', () => {
    const fixture = build();

    expect(fixture.componentInstance.hasNothingToChoose()).toBe(false);
    expect(heading(fixture)).toBe('Select an organization');
  });

  // Signing in creates no organization, so a first-time user has nothing to
  // pick from until an operator adds them. That is a message, not a blank page.
  it('tells someone in no organization that an operator has to add them', () => {
    const fixture = build([], []);

    expect(fixture.componentInstance.hasNothingToChoose()).toBe(true);
    expect(heading(fixture)).toBe('You are not in an organization yet');
    // Naming the account makes the request to the operator a concrete one, and
    // the id is what `funops organization member add` accepts.
    const text = (fixture.nativeElement as HTMLElement).textContent;
    expect(text).toContain('Alice');
    expect(text).toContain('019b4000-1000-7000-8000-000000000001');
  });

  it('keeps offering an invitation to someone who is otherwise in no organization', () => {
    // The invited organization is in the list too, as a pending one.
    const invited = create(OrganizationSchema, { id: 'org-2', name: 'globex', alias: 'Globex' });
    const fixture = build([invited], [INVITATION]);

    expect(fixture.componentInstance.hasNothingToChoose()).toBe(false);
    expect(fixture.componentInstance.acceptedOrganizations()).toEqual([]);
    expect(heading(fixture)).toBe('Select an organization');
  });

  it('asks the shell to look again when someone in no organization checks', () => {
    const fixture = build([], []);
    let rechecks = 0;
    fixture.componentInstance.recheck.subscribe(() => {
      rechecks += 1;
    });

    fixture.componentInstance.onRecheck();

    expect(rechecks).toBe(1);
  });
});
