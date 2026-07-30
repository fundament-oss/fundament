import { mountCreateForm } from './create-scaffold.ts';
import { buildHTTPRouteBody } from './httproutes-form.ts';
import { validateRoute } from './form-helpers.ts';

await mountCreateForm({
  intro: 'Route HTTP traffic from a Gateway to a backend Service.',
  resource: { group: 'gateway.networking.k8s.io', version: 'v1', resource: 'httproutes' },
  buildBody: buildHTTPRouteBody,
  validate: validateRoute,
});
