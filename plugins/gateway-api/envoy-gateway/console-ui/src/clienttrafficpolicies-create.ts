import { mountCreateForm } from './create-scaffold.ts';
import { buildClientTrafficPolicyBody } from './clienttrafficpolicies-form.ts';
import { validatePolicy } from './form-helpers.ts';

await mountCreateForm({
  intro:
    'Apply a ClientTrafficPolicy to a Gateway (e.g. downstream TLS minimum version). Connection and timeout tuning is configured via YAML.',
  resource: {
    group: 'gateway.envoyproxy.io',
    version: 'v1alpha1',
    resource: 'clienttrafficpolicies',
  },
  buildBody: buildClientTrafficPolicyBody,
  validate: validatePolicy,
});
