import { describe, expect, it } from 'vitest';
import { buildTLSRouteBody } from './tlsroutes-form.ts';

function renderForm(o: { name?: string; parent?: string; hostnames?: string; backend?: string; port?: string }): HTMLFormElement {
  document.body.innerHTML = `
    <form id="form">
      <input id="name" value="${o.name ?? 'tls'}" />
      <input id="parent-name" value="${o.parent ?? 'demo'}" />
      <input id="parent-section" value="" />
      <input id="hostnames" value="${o.hostnames ?? ''}" />
      <input id="backend-name" value="${o.backend ?? 'secure'}" />
      <input id="backend-port" value="${o.port ?? '8443'}" />
    </form>`;
  return document.getElementById('form') as HTMLFormElement;
}

describe('buildTLSRouteBody', () => {
  it('builds a v1alpha2 TLSRoute with SNI hostnames and one backend', () => {
    const body = buildTLSRouteBody(renderForm({ name: 'tls', hostnames: 'secure.example.com', backend: 'secure', port: '8443' }), 'team-a');

    expect(body.apiVersion).toBe('gateway.networking.k8s.io/v1alpha2');
    expect(body.kind).toBe('TLSRoute');
    expect(body.spec.parentRefs).toEqual([{ name: 'demo' }]);
    expect(body.spec.hostnames).toEqual(['secure.example.com']);
    expect(body.spec.rules).toEqual([{ backendRefs: [{ name: 'secure', port: 8443 }] }]);
  });

  it('omits hostnames when none given', () => {
    const body = buildTLSRouteBody(renderForm({ hostnames: '' }), 'team-a');
    expect(body.spec).not.toHaveProperty('hostnames');
  });
});
