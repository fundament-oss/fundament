import { describe, expect, it } from 'vitest';
import { buildSecurityPolicyBody } from './securitypolicies-form.ts';
import { validatePolicy } from './form-helpers.ts';

function renderForm(o: {
  name?: string;
  kind?: string;
  target?: string;
  origins?: string;
  methods?: string;
}): HTMLFormElement {
  document.body.innerHTML = `
    <form id="form">
      <input id="name" value="${o.name ?? 'sec'}" />
      <input id="target-kind" value="${o.kind ?? 'Gateway'}" />
      <input id="target-name" value="${o.target ?? 'demo'}" />
      <input id="cors-origins" value="${o.origins ?? ''}" />
      <input id="cors-methods" value="${o.methods ?? ''}" />
    </form>`;
  return document.getElementById('form') as HTMLFormElement;
}

describe('buildSecurityPolicyBody', () => {
  it('targets a Gateway and omits cors when no origins/methods', () => {
    const body = buildSecurityPolicyBody(
      renderForm({ name: 'sec', target: 'demo' }),
      'team-a',
    );

    expect(body.apiVersion).toBe('gateway.envoyproxy.io/v1alpha1');
    expect(body.kind).toBe('SecurityPolicy');
    expect(body.spec.targetRefs).toEqual([
      { group: 'gateway.networking.k8s.io', kind: 'Gateway', name: 'demo' },
    ]);
    expect(body.spec).not.toHaveProperty('cors');
  });

  it('adds a cors block from allowOrigins and allowMethods', () => {
    const body = buildSecurityPolicyBody(
      renderForm({
        kind: 'HTTPRoute',
        target: 'web',
        origins: 'https://a.com, https://b.com',
        methods: 'GET, POST',
      }),
      'team-a',
    );

    expect(body.spec.targetRefs[0].kind).toBe('HTTPRoute');
    expect(body.spec.cors).toEqual({
      allowOrigins: ['https://a.com', 'https://b.com'],
      allowMethods: ['GET', 'POST'],
    });
  });
});

describe('validatePolicy (SecurityPolicy)', () => {
  it('requires a name and a target', () => {
    expect(validatePolicy(renderForm({}))).toBe(true);
    expect(validatePolicy(renderForm({ target: '' }))).toBe(false);
    expect(validatePolicy(renderForm({ name: '' }))).toBe(false);
  });
});
