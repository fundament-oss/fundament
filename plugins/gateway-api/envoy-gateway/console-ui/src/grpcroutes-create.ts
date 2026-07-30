import { mountCreateForm } from './create-scaffold.ts';
import { buildGRPCRouteBody } from './grpcroutes-form.ts';
import { validateRoute } from './form-helpers.ts';

await mountCreateForm({
  intro: 'Route gRPC traffic from a Gateway to a backend Service.',
  resource: { group: 'gateway.networking.k8s.io', version: 'v1', resource: 'grpcroutes' },
  buildBody: buildGRPCRouteBody,
  validate: validateRoute,
});
