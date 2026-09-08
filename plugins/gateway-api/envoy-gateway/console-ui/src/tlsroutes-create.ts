import { mountCreateForm } from './create-scaffold.ts';
import { buildTLSRouteBody } from './tlsroutes-form.ts';
import { validateRoute } from './form-helpers.ts';

await mountCreateForm({
  intro:
    'Route TLS connections by SNI hostname from a Gateway to a backend Service.',
  resource: {
    group: 'gateway.networking.k8s.io',
    version: 'v1alpha2',
    resource: 'tlsroutes',
  },
  buildBody: buildTLSRouteBody,
  validate: validateRoute,
});
