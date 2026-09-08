import { describe, expect, it } from 'vitest';
import { buildBackendTrafficPolicyBody } from './backendtrafficpolicies-form.ts';

function renderForm(o: {
  name?: string;
  kind?: string;
  target?: string;
  requests?: string;
  unit?: string;
}): HTMLFormElement {
  document.body.innerHTML = `
    <form id="form">
      <input id="name" value="${o.name ?? 'btp'}" />
      <input id="target-kind" value="${o.kind ?? 'Gateway'}" />
      <input id="target-name" value="${o.target ?? 'demo'}" />
      <input id="rate-requests" value="${o.requests ?? ''}" />
      <input id="rate-unit" value="${o.unit ?? ''}" />
    </form>`;
  return document.getElementById('form') as HTMLFormElement;
}

describe('buildBackendTrafficPolicyBody', () => {
  it('omits rateLimit when no request count', () => {
    const body = buildBackendTrafficPolicyBody(
      renderForm({ target: 'demo' }),
      'team-a',
    );

    expect(body.apiVersion).toBe('gateway.envoyproxy.io/v1alpha1');
    expect(body.kind).toBe('BackendTrafficPolicy');
    expect(body.spec.targetRefs).toEqual([
      { group: 'gateway.networking.k8s.io', kind: 'Gateway', name: 'demo' },
    ]);
    expect(body.spec).not.toHaveProperty('rateLimit');
  });

  it('builds a local rate limit with the chosen unit', () => {
    const body = buildBackendTrafficPolicyBody(
      renderForm({ requests: '100', unit: 'Minute' }),
      'team-a',
    );
    expect(body.spec.rateLimit).toEqual({
      type: 'Local',
      local: { rules: [{ limit: { requests: 100, unit: 'Minute' } }] },
    });
  });

  it('defaults the unit to Second', () => {
    const body = buildBackendTrafficPolicyBody(
      renderForm({ requests: '5' }),
      'team-a',
    );
    expect(body.spec.rateLimit!.local.rules[0].limit.unit).toBe('Second');
  });
});
