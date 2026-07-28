import { mountCreateForm } from './create-scaffold.ts';
import { buildTCPRouteBody } from './tcproutes-form.ts';
import { validateRoute } from './form-helpers.ts';

await mountCreateForm({
  intro: 'Forward TCP traffic from a Gateway listener to a backend Service.',
  resource: { group: 'gateway.networking.k8s.io', version: 'v1alpha2', resource: 'tcproutes' },
  buildBody: buildTCPRouteBody,
  validate: validateRoute,
});
