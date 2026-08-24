import { describe, expect, it } from 'vitest';
import { buildGRPCRouteBody } from './grpcroutes-form.ts';

function renderForm(o: { name?: string; parent?: string; hostnames?: string; backend?: string; port?: string }): HTMLFormElement {
  document.body.innerHTML = `
    <form id="form">
      <input id="name" value="${o.name ?? 'grpc'}" />
      <input id="parent-name" value="${o.parent ?? 'demo'}" />
      <input id="parent-section" value="" />
      <input id="hostnames" value="${o.hostnames ?? ''}" />
      <input id="backend-name" value="${o.backend ?? 'grpc-svc'}" />
      <input id="backend-port" value="${o.port ?? '9000'}" />
    </form>`;
  return document.getElementById('form') as HTMLFormElement;
}

describe('buildGRPCRouteBody', () => {
  it('builds a GRPCRoute forwarding to one backend', () => {
    const body = buildGRPCRouteBody(renderForm({ name: 'grpc', backend: 'grpc-svc', port: '9000' }), 'team-a');

    expect(body.apiVersion).toBe('gateway.networking.k8s.io/v1');
    expect(body.kind).toBe('GRPCRoute');
    expect(body.metadata).toEqual({ name: 'grpc', namespace: 'team-a' });
    expect(body.spec.parentRefs).toEqual([{ name: 'demo' }]);
    expect(body.spec.rules).toEqual([{ backendRefs: [{ name: 'grpc-svc', port: 9000 }] }]);
    expect(body.spec).not.toHaveProperty('hostnames');
  });

  it('includes hostnames when provided', () => {
    const body = buildGRPCRouteBody(renderForm({ hostnames: 'grpc.example.com' }), 'team-a');
    expect(body.spec.hostnames).toEqual(['grpc.example.com']);
  });
});
