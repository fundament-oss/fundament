// Tests for the Gateway create-form logic. The NLDD Design System is not loaded
// here, so <nldd-*> are unknown elements; the harness reflects declared
// attributes onto the same-named properties the form reads (.value / .checked),
// mirroring the part of Lit the form depends on.

import { describe, expect, it } from 'vitest';
import { buildGatewayBody, validateForm } from './form.ts';

function upgrade(root: ParentNode): void {
  root.querySelectorAll('nldd-text-field, nldd-checkbox-field').forEach((el) => {
    const v = el.getAttribute('value');
    if (v !== null) (el as unknown as { value: string }).value = v;
    const checked = el.hasAttribute('checked');
    (el as unknown as { checked: boolean }).checked = checked;
    if (el.hasAttribute('required')) (el as unknown as { required: boolean }).required = true;
  });
}

function renderForm(overrides: {
  name?: string;
  https?: boolean;
  tlsMode?: 'secret' | 'certManager';
  tlsSecret?: string;
  issuer?: string;
}): HTMLFormElement {
  document.body.innerHTML = `
    <form id="form">
      <nldd-text-field id="name" required value="${overrides.name ?? 'web'}"></nldd-text-field>
      <nldd-checkbox-field id="https-enabled" ${overrides.https ? 'checked' : ''}></nldd-checkbox-field>
      <select id="tls-mode"><option value="secret" ${overrides.tlsMode === 'secret' ? 'selected' : ''}>secret</option><option value="certManager" ${overrides.tlsMode === 'certManager' ? 'selected' : ''}>certManager</option></select>
      <nldd-text-field id="tls-secret" value="${overrides.tlsSecret ?? ''}"></nldd-text-field>
      <nldd-text-field id="cluster-issuer" value="${overrides.issuer ?? ''}"></nldd-text-field>
    </form>`;
  const form = document.getElementById('form') as HTMLFormElement;
  upgrade(form);
  return form;
}

describe('buildGatewayBody', () => {
  it('builds an HTTP-only Gateway with the eg class and All allowedRoutes', () => {
    const body = buildGatewayBody(renderForm({ name: 'web', https: false }), 'team-a');

    expect(body.apiVersion).toBe('gateway.networking.k8s.io/v1');
    expect(body.kind).toBe('Gateway');
    expect(body.metadata).toEqual({ name: 'web', namespace: 'team-a' });
    expect(body.spec.gatewayClassName).toBe('eg');
    expect(body.spec.listeners).toHaveLength(1);
    expect(body.spec.listeners[0]).toMatchObject({
      name: 'http',
      protocol: 'HTTP',
      port: 80,
      allowedRoutes: { namespaces: { from: 'All' } },
    });
    expect(body.metadata).not.toHaveProperty('annotations');
  });

  it('adds an HTTPS listener referencing a TLS secret', () => {
    const body = buildGatewayBody(
      renderForm({ https: true, tlsMode: 'secret', tlsSecret: 'web-tls' }),
      'team-a',
    );

    expect(body.spec.listeners).toHaveLength(2);
    expect(body.spec.listeners[1]).toMatchObject({
      name: 'https',
      protocol: 'HTTPS',
      port: 443,
      tls: { mode: 'Terminate', certificateRefs: [{ name: 'web-tls' }] },
    });
    expect(body.metadata).not.toHaveProperty('annotations');
  });

  it('adds a cert-manager cluster-issuer annotation and derives the secret name', () => {
    const body = buildGatewayBody(
      renderForm({ name: 'web', https: true, tlsMode: 'certManager', issuer: 'letsencrypt' }),
      'team-a',
    );

    expect((body.metadata as Record<string, unknown>).annotations).toEqual({
      'cert-manager.io/cluster-issuer': 'letsencrypt',
    });
    // Non-null assertion: HTTPS is enabled, so this listener always carries tls.
    expect(body.spec.listeners[1].tls!.certificateRefs).toEqual([{ name: 'web-tls' }]);
  });
});

describe('validateForm', () => {
  it('fails when the name is empty', () => {
    expect(validateForm(renderForm({ name: '' }))).toBe(false);
  });
  it('fails when HTTPS+secret is chosen but no secret name given', () => {
    expect(validateForm(renderForm({ https: true, tlsMode: 'secret', tlsSecret: '' }))).toBe(false);
  });
  it('fails when HTTPS+certManager is chosen but no issuer given', () => {
    expect(validateForm(renderForm({ https: true, tlsMode: 'certManager', issuer: '' }))).toBe(false);
  });
  it('passes for a valid HTTP-only form', () => {
    expect(validateForm(renderForm({ name: 'web', https: false }))).toBe(true);
  });
});
