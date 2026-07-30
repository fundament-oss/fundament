import { describe, expect, it } from 'vitest';
import { buildHTTPRouteBody } from './httproutes-form.ts';
import { validateRoute } from './form-helpers.ts';

function renderForm(o: {
  name?: string;
  parent?: string;
  section?: string;
  hostnames?: string;
  path?: string;
  backend?: string;
  port?: string;
}): HTMLFormElement {
  document.body.innerHTML = `
    <form id="form">
      <input id="name" value="${o.name ?? 'web'}" />
      <input id="parent-name" value="${o.parent ?? 'demo'}" />
      <input id="parent-section" value="${o.section ?? ''}" />
      <input id="hostnames" value="${o.hostnames ?? ''}" />
      <input id="path" value="${o.path ?? ''}" />
      <input id="backend-name" value="${o.backend ?? 'web'}" />
      <input id="backend-port" value="${o.port ?? '80'}" />
    </form>`;
  return document.getElementById('form') as HTMLFormElement;
}

describe('buildHTTPRouteBody', () => {
  it('builds a route attaching to a Gateway with a default "/" path and one backend', () => {
    const body = buildHTTPRouteBody(renderForm({ name: 'web', parent: 'demo', backend: 'echo', port: '8080' }), 'team-a');

    expect(body.apiVersion).toBe('gateway.networking.k8s.io/v1');
    expect(body.kind).toBe('HTTPRoute');
    expect(body.metadata).toEqual({ name: 'web', namespace: 'team-a' });
    expect(body.spec.parentRefs).toEqual([{ name: 'demo' }]);
    expect(body.spec).not.toHaveProperty('hostnames');
    expect(body.spec.rules).toEqual([
      {
        matches: [{ path: { type: 'PathPrefix', value: '/' } }],
        backendRefs: [{ name: 'echo', port: 8080 }],
      },
    ]);
  });

  it('includes hostnames and a sectionName when provided', () => {
    const body = buildHTTPRouteBody(
      renderForm({ parent: 'demo', section: 'http', hostnames: 'a.example.com, b.example.com', path: '/api' }),
      'team-a',
    );

    expect(body.spec.parentRefs).toEqual([{ name: 'demo', sectionName: 'http' }]);
    expect(body.spec.hostnames).toEqual(['a.example.com', 'b.example.com']);
    expect(body.spec.rules[0].matches[0].path.value).toBe('/api');
  });
});

describe('validateRoute (HTTPRoute)', () => {
  it('passes with name, parent and backend', () => {
    expect(validateRoute(renderForm({}))).toBe(true);
  });
  it('fails without a parent Gateway', () => {
    expect(validateRoute(renderForm({ parent: '' }))).toBe(false);
  });
  it('fails with a non-numeric port', () => {
    expect(validateRoute(renderForm({ port: 'abc' }))).toBe(false);
  });
});
