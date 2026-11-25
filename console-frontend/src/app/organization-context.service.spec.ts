import { TestBed } from '@angular/core/testing';
import { afterEach, describe, expect, it } from 'vitest';
import OrganizationContextService from './organization-context.service';

describe('OrganizationContextService.organizationIdSet', () => {
  afterEach(() => localStorage.clear());

  it('resolves at once when an id is set', async () => {
    const context = TestBed.inject(OrganizationContextService);
    context.setOrganizationId('org-1');

    await expect(context.organizationIdSet()).resolves.toBe('org-1');
  });

  it('resolves with the next id that is set', async () => {
    const context = TestBed.inject(OrganizationContextService);
    const wait = context.organizationIdSet();

    context.setOrganizationId('org-1');

    await expect(wait).resolves.toBe('org-1');
  });

  it('rejects a wait when the id is cleared, so a later id does not answer it', async () => {
    const context = TestBed.inject(OrganizationContextService);
    const cleared = context.organizationIdSet();

    context.setOrganizationId(null);
    await expect(cleared).rejects.toThrow('organization cleared');

    // Sign-out: a wait from this session is not answered by the next one.
    const signedOut = context.organizationIdSet();
    context.clearOrganizationId();
    context.setOrganizationId('org-2');
    await expect(signedOut).rejects.toThrow('organization cleared');
  });

  it('rejects a wait whose signal aborts, and drops it', async () => {
    const context = TestBed.inject(OrganizationContextService);
    const controller = new AbortController();
    const wait = context.organizationIdSet(controller.signal);

    controller.abort(new Error('gave up'));
    await expect(wait).rejects.toThrow('gave up');

    // Settled already: a later id is not handed to it.
    context.setOrganizationId('org-1');
    await expect(wait).rejects.toThrow('gave up');
  });

  it('rejects at once for a signal that aborted before the call', async () => {
    const context = TestBed.inject(OrganizationContextService);
    const controller = new AbortController();
    controller.abort(new Error('gave up'));

    await expect(context.organizationIdSet(controller.signal)).rejects.toThrow('gave up');
  });
});
