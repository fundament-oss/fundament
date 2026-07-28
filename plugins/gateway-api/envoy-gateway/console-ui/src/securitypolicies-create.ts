import { mountCreateForm } from './create-scaffold.ts';
import { buildSecurityPolicyBody } from './securitypolicies-form.ts';
import { validatePolicy } from './form-helpers.ts';

await mountCreateForm({
  intro: 'Apply a SecurityPolicy (e.g. CORS) to a Gateway or route. Advanced auth features are configured via YAML.',
  resource: { group: 'gateway.envoyproxy.io', version: 'v1alpha1', resource: 'securitypolicies' },
  buildBody: buildSecurityPolicyBody,
  validate: validatePolicy,
});
