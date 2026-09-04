import { describe, expect, it } from 'vitest';
import { buildTCPRouteBody } from './tcproutes-form.ts';

function renderForm(o: {
  name?: string;
  parent?: string;
  section?: string;
  backend?: string;
  port?: string;
}): HTMLFormElement {
  document.body.innerHTML = `
    <form id="form">
      <input id="name" value="${o.name ?? 'tcp'}" />
      <input id="parent-name" value="${o.parent ?? 'demo'}" />
      <input id="parent-section" value="${o.section ?? ''}" />
      <input id="backend-name" value="${o.backend ?? 'db'}" />
      <input id="backend-port" value="${o.port ?? '5432'}" />
    </form>`;
  return document.getElementById('form') as HTMLFormElement;
}

describe('buildTCPRouteBody', () => {
  it('builds a v1alpha2 TCPRoute with parent and backend', () => {
    const body = buildTCPRouteBody(
      renderForm({
        name: 'tcp',
        section: 'postgres',
        backend: 'db',
        port: '5432',
      }),
      'team-a',
    );

    expect(body.apiVersion).toBe('gateway.networking.k8s.io/v1alpha2');
    expect(body.kind).toBe('TCPRoute');
    expect(body.metadata).toEqual({ name: 'tcp', namespace: 'team-a' });
    expect(body.spec.parentRefs).toEqual([
      { name: 'demo', sectionName: 'postgres' },
    ]);
    expect(body.spec.rules).toEqual([
      { backendRefs: [{ name: 'db', port: 5432 }] },
    ]);
  });
});
