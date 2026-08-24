import { mountCreateForm } from './create-scaffold.ts';
import { buildBackendTrafficPolicyBody } from './backendtrafficpolicies-form.ts';
import { validatePolicy } from './form-helpers.ts';

await mountCreateForm({
  intro: 'Apply a BackendTrafficPolicy (e.g. a local rate limit) to a Gateway or route. Load balancing, retries and circuit breaking are configured via YAML.',
  resource: { group: 'gateway.envoyproxy.io', version: 'v1alpha1', resource: 'backendtrafficpolicies' },
  buildBody: buildBackendTrafficPolicyBody,
  validate: validatePolicy,
});
