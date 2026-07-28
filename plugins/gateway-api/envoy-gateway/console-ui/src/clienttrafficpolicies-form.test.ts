import { describe, expect, it } from 'vitest';
import { buildClientTrafficPolicyBody } from './clienttrafficpolicies-form.ts';

function renderForm(o: { name?: string; target?: string; minVersion?: string }): HTMLFormElement {
  document.body.innerHTML = `
    <form id="form">
      <input id="name" value="${o.name ?? 'ctp'}" />
      <input id="target-kind" value="Gateway" />
      <input id="target-name" value="${o.target ?? 'demo'}" />
      <input id="tls-min-version" value="${o.minVersion ?? ''}" />
    </form>`;
  return document.getElementById('form') as HTMLFormElement;
}

describe('buildClientTrafficPolicyBody', () => {
  it('targets a Gateway and omits tls when no min version', () => {
    const body = buildClientTrafficPolicyBody(renderForm({ target: 'demo' }), 'team-a');

    expect(body.apiVersion).toBe('gateway.envoyproxy.io/v1alpha1');
    expect(body.kind).toBe('ClientTrafficPolicy');
    expect(body.spec.targetRefs).toEqual([{ group: 'gateway.networking.k8s.io', kind: 'Gateway', name: 'demo' }]);
    expect(body.spec).not.toHaveProperty('tls');
  });

  it('sets tls.minVersion when chosen', () => {
    const body = buildClientTrafficPolicyBody(renderForm({ minVersion: '1.3' }), 'team-a');
    expect(body.spec.tls).toEqual({ minVersion: '1.3' });
  });
});
