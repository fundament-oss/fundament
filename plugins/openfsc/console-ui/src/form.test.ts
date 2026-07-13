// Tests for the create-form logic.
//
// The NLDD Design System itself is not loaded here (it needs a real custom-element
// registry and the host's /plugin-ui/nldd-design-system.js), so the <nldd-*> tags are unknown
// elements. `upgrade` below stands in for the part of Lit the form code actually
// depends on: reflecting the declared attributes (value / checked / required) onto
// properties of the same name. Keeping that reflection explicit is deliberate — the
// form reads `.value` and `.checked` as properties, and a harness that quietly
// diverged from the real components would make these tests attest to the wrong thing.

import { beforeEach, describe, expect, it } from 'vitest';
import {
  applyMode,
  buildBody,
  gatewayRowHtml,
  gatherGateways,
  namespaceFieldHtml,
  trimmedValue,
  validateForm,
  validateTextField,
} from './form.ts';
import type { NlddTextField } from './nldd-design-system.ts';

// A minimal stand-in for the real create form: the ids buildBody reads, plus the
// External fieldset and the gateway containers.
function renderForm(): HTMLFormElement {
  document.body.innerHTML = `
    <form id="form">
      <nldd-text-field id="name" required maxlength="63"
        pattern="[a-z0-9]([a-z0-9\\-]*[a-z0-9])?"
        data-error="Use lowercase letters, digits and dashes."
        error-message="name-error"></nldd-text-field>
      <nldd-form-field-error-text id="name-error">Enter a valid name.</nldd-form-field-error-text>

      <div id="namespace-field"></div>

      <nldd-text-field id="groupID" required></nldd-text-field>
      <nldd-text-field id="peerID" required></nldd-text-field>

      <select id="mode" name="mode">
        <option value="Self" selected>Self</option>
        <option value="External">External</option>
      </select>

      <fieldset id="external-fieldset" hidden>
        <nldd-text-field id="ext-address" data-external-required
          pattern="https://.+" data-error="Must start with https://"></nldd-text-field>
        <nldd-text-field id="ext-peerID" data-external-required></nldd-text-field>
        <nldd-text-field id="ext-ta-name" data-external-required></nldd-text-field>
        <nldd-text-field id="ext-ta-key"></nldd-text-field>
        <nldd-text-field id="certificate" data-external-required></nldd-text-field>
      </fieldset>

      <nldd-text-field id="managerAddress" pattern="https://.+"></nldd-text-field>
      <nldd-text-field id="controllerURL" pattern="https?://.+"></nldd-text-field>

      <nldd-checkbox-field name="autoSignGrants" value="serviceConnection"></nldd-checkbox-field>
      <nldd-checkbox-field name="autoSignGrants" value="servicePublication"></nldd-checkbox-field>

      <nldd-text-field id="pg-instances" type="number" min="1"></nldd-text-field>
      <nldd-text-field id="pg-storageClass" required></nldd-text-field>
      <nldd-text-field id="pg-storageSize"></nldd-text-field>
      <nldd-text-field id="pg-image"></nldd-text-field>

      <div id="inways"></div>
      <div id="outways"></div>
    </form>`;
  const form = document.getElementById('form') as HTMLFormElement;
  upgrade(form);
  return form;
}

// Stands in for Lit's attribute→property reflection on the NLDD Design System
// elements in `root`: `value`, `checked` and `required` are declared reactive
// properties, so the real
// components expose them whether or not the attribute was present. The form code
// reads them as properties, so the harness must too.
function upgrade(root: ParentNode): void {
  root.querySelectorAll('nldd-text-field, nldd-checkbox-field').forEach((node) => {
    const el = node as unknown as { value: string; checked: boolean; required: boolean };
    el.value = node.getAttribute('value') ?? '';
    el.checked = node.hasAttribute('checked');
    el.required = node.hasAttribute('required');
  });
}

// Sets a control's value the way a user typing into it would.
function setValue(form: HTMLFormElement, id: string, value: string): void {
  (form.querySelector(`#${id}`) as unknown as { value: string }).value = value;
}

function check(form: HTMLFormElement, value: string): void {
  (
    form.querySelector(`nldd-checkbox-field[value="${value}"]`) as unknown as { checked: boolean }
  ).checked = true;
}

// Fills in everything a valid Self-mode installation needs.
function fillRequired(form: HTMLFormElement): void {
  setValue(form, 'name', 'my-installation');
  setValue(form, 'groupID', 'fsc-demo');
  setValue(form, 'peerID', '12345678901234567890');
  setValue(form, 'pg-storageClass', 'local-path');
}

function addGatewayRow(form: HTMLFormElement, kind: 'inway' | 'outway', seq = 0): HTMLElement {
  const row = document.createElement('div');
  row.className = 'plugin-row gateway-row';
  row.innerHTML = gatewayRowHtml(kind, seq);
  upgrade(row);
  form.querySelector(`#${kind}s`)!.appendChild(row);
  return row;
}

let form: HTMLFormElement;
beforeEach(() => {
  form = renderForm();
});

describe('applyMode', () => {
  it('hides the external fieldset and clears its required flags in Self mode', () => {
    setValue(form, 'mode', 'External');
    applyMode(form);
    setValue(form, 'mode', 'Self');
    applyMode(form);

    expect((form.querySelector('#external-fieldset') as HTMLElement).hidden).toBe(true);
    form.querySelectorAll<NlddTextField>('[data-external-required]').forEach((el) => {
      expect(el.required).toBe(false);
    });
  });

  it('reveals the external fieldset and requires its fields in External mode', () => {
    setValue(form, 'mode', 'External');
    applyMode(form);

    expect((form.querySelector('#external-fieldset') as HTMLElement).hidden).toBe(false);
    form.querySelectorAll<NlddTextField>('[data-external-required]').forEach((el) => {
      expect(el.required).toBe(true);
    });
  });

  it('hides per-gateway certificate fields in Self mode and shows them in External', () => {
    addGatewayRow(form, 'inway');

    applyMode(form); // Self
    expect((form.querySelector('.gw-cert-field') as HTMLElement).hidden).toBe(true);

    setValue(form, 'mode', 'External');
    applyMode(form);
    expect((form.querySelector('.gw-cert-field') as HTMLElement).hidden).toBe(false);
  });
});

describe('validateTextField', () => {
  const field = (attrs: string, value: string): NlddTextField => {
    document.body.innerHTML = `<div><nldd-text-field ${attrs}></nldd-text-field></div>`;
    upgrade(document.body);
    const el = document.querySelector('nldd-text-field') as NlddTextField;
    el.value = value;
    return el;
  };

  it('rejects an empty required field and marks it invalid', () => {
    const el = field('required', '  ');
    expect(validateTextField(el)).toBe(false);
    expect(el.hasAttribute('invalid')).toBe(true);
  });

  it('accepts an empty optional field', () => {
    expect(validateTextField(field('pattern="https://.+"', ''))).toBe(true);
  });

  it('anchors the pattern so a partial match is rejected', () => {
    // Unanchored, /https:\/\/.+/ would match anywhere in the string.
    expect(validateTextField(field('pattern="https://.+"', 'ftp://x https://y'))).toBe(false);
    expect(validateTextField(field('pattern="https://.+"', 'https://ok.example'))).toBe(true);
  });

  it('enforces maxlength', () => {
    expect(validateTextField(field('maxlength="5"', '123456'))).toBe(false);
    expect(validateTextField(field('maxlength="5"', '12345'))).toBe(true);
  });

  it('enforces integer and min for number fields', () => {
    expect(validateTextField(field('type="number" min="1"', '1.5'))).toBe(false);
    expect(validateTextField(field('type="number" min="1"', '0'))).toBe(false);
    expect(validateTextField(field('type="number" min="1"', '3'))).toBe(true);
  });

  it('skips fields inside a hidden ancestor', () => {
    document.body.innerHTML = `
      <fieldset hidden><nldd-text-field id="x" required></nldd-text-field></fieldset>`;
    upgrade(document.body);
    const el = document.querySelector('#x') as NlddTextField;

    // Required but empty — yet hidden, so not part of the submission.
    expect(validateTextField(el)).toBe(true);
    expect(el.hasAttribute('invalid')).toBe(false);
  });

  it('writes the data-error message into the linked error-text element', () => {
    document.body.innerHTML = `
      <div>
        <nldd-text-field id="n" pattern="[a-z]+" data-error="Lowercase only."
          error-message="n-error"></nldd-text-field>
        <nldd-form-field-error-text id="n-error">placeholder</nldd-form-field-error-text>
      </div>`;
    upgrade(document.body);
    const el = document.querySelector('#n') as NlddTextField;
    el.value = 'NOPE';

    expect(validateTextField(el)).toBe(false);
    expect(document.getElementById('n-error')!.textContent).toBe('Lowercase only.');
  });
});

describe('validateForm', () => {
  it('passes once the required Self-mode fields are filled', () => {
    fillRequired(form);
    applyMode(form);
    expect(validateForm(form)).toBe(true);
  });

  it('fails when a required field is blank', () => {
    fillRequired(form);
    setValue(form, 'pg-storageClass', '');
    applyMode(form);
    expect(validateForm(form)).toBe(false);
  });

  it('ignores the External fields while in Self mode', () => {
    fillRequired(form);
    applyMode(form); // Self → external fieldset hidden, its fields not required

    // ext-address is blank and would be invalid in External mode.
    expect(validateForm(form)).toBe(true);
  });

  it('enforces the External fields once External mode is selected', () => {
    fillRequired(form);
    setValue(form, 'mode', 'External');
    applyMode(form);

    expect(validateForm(form)).toBe(false);

    setValue(form, 'ext-address', 'https://directory.example.com');
    setValue(form, 'ext-peerID', '09876543210987654321');
    setValue(form, 'ext-ta-name', 'group-ca');
    setValue(form, 'certificate', 'my-group-cert');
    expect(validateForm(form)).toBe(true);
  });

  it('rejects a name that is not a valid k8s-style identifier', () => {
    fillRequired(form);
    setValue(form, 'name', 'Not_A_Valid_Name');
    applyMode(form);
    expect(validateForm(form)).toBe(false);
  });
});

describe('buildBody', () => {
  it('builds a minimal Self-mode installation without empty optional keys', () => {
    fillRequired(form);
    applyMode(form);

    const body = buildBody(form, 'fsc-demo');

    expect(body).toEqual({
      apiVersion: 'openfsc.fundament.io/v1',
      kind: 'FSCInstallation',
      metadata: { name: 'my-installation', namespace: 'fsc-demo' },
      spec: {
        groupID: 'fsc-demo',
        peerID: '12345678901234567890',
        directory: { mode: 'Self' },
        postgres: { storageClass: 'local-path' },
      },
    });
    // Nothing the user left blank should leak into the CR.
    expect(body.spec).not.toHaveProperty('managerAddress');
    expect(body.spec).not.toHaveProperty('controllerURL');
    expect(body.spec).not.toHaveProperty('certificate');
    expect(body.spec).not.toHaveProperty('autoSignGrants');
  });

  it('omits directory.external in Self mode even when the fields hold stale values', () => {
    fillRequired(form);
    // The user switched to External, typed, then switched back to Self.
    setValue(form, 'ext-address', 'https://stale.example.com');
    setValue(form, 'certificate', 'stale-cert');
    setValue(form, 'mode', 'Self');
    applyMode(form);

    const body = buildBody(form, 'fsc-demo');

    expect(body.spec.directory).toEqual({ mode: 'Self' });
    expect(body.spec).not.toHaveProperty('certificate');
  });

  it('includes directory.external in External mode and defaults the trust-anchor key', () => {
    fillRequired(form);
    setValue(form, 'mode', 'External');
    setValue(form, 'ext-address', 'https://directory.example.com');
    setValue(form, 'ext-peerID', '09876543210987654321');
    setValue(form, 'ext-ta-name', 'group-ca');
    setValue(form, 'certificate', 'my-group-cert');
    applyMode(form);

    const body = buildBody(form, 'fsc-demo');

    expect(body.spec.directory).toEqual({
      mode: 'External',
      external: {
        address: 'https://directory.example.com',
        peerID: '09876543210987654321',
        trustAnchor: { name: 'group-ca', key: 'ca.crt' },
      },
    });
    expect(body.spec.certificate).toEqual({ existingSecret: 'my-group-cert' });
  });

  it('honours an explicit trust-anchor key', () => {
    fillRequired(form);
    setValue(form, 'mode', 'External');
    setValue(form, 'ext-ta-name', 'group-ca');
    setValue(form, 'ext-ta-key', 'root.pem');
    applyMode(form);

    const external = (body: unknown) =>
      (body as { spec: { directory: { external: { trustAnchor: { key: string } } } } }).spec
        .directory.external.trustAnchor.key;

    expect(external(buildBody(form, 'fsc-demo'))).toBe('root.pem');
  });

  it('coerces the postgres instance count to a number', () => {
    fillRequired(form);
    setValue(form, 'pg-instances', '3');
    applyMode(form);

    expect(buildBody(form, 'fsc-demo').spec.postgres).toEqual({
      storageClass: 'local-path',
      instances: 3,
    });
  });

  it('collects only the checked auto-sign grants', () => {
    fillRequired(form);
    check(form, 'servicePublication');
    applyMode(form);

    expect(buildBody(form, 'fsc-demo').spec.autoSignGrants).toEqual(['servicePublication']);
  });

  it('trims whitespace out of every value', () => {
    fillRequired(form);
    setValue(form, 'name', '  my-installation  ');
    setValue(form, 'groupID', '  fsc-demo  ');
    applyMode(form);

    const body = buildBody(form, 'fsc-demo');
    expect(body.metadata.name).toBe('my-installation');
    expect(body.spec.groupID).toBe('fsc-demo');
  });
});

describe('gatherGateways', () => {
  it('returns an inway with its self address', () => {
    const row = addGatewayRow(form, 'inway');
    (row.querySelector('.gw-name') as NlddTextField).value = 'default';
    (row.querySelector('.gw-self') as NlddTextField).value = 'https://inway.example.com';

    expect(gatherGateways(form, 'inway', false)).toEqual([
      { name: 'default', selfAddress: 'https://inway.example.com' },
    ]);
  });

  it('drops rows with no name', () => {
    addGatewayRow(form, 'outway', 0);
    const named = addGatewayRow(form, 'outway', 1);
    (named.querySelector('.gw-name') as NlddTextField).value = 'consumer';

    expect(gatherGateways(form, 'outway', false)).toEqual([{ name: 'consumer' }]);
  });

  it('omits the per-gateway certificate in Self mode, where it is not valid', () => {
    const row = addGatewayRow(form, 'inway');
    (row.querySelector('.gw-name') as NlddTextField).value = 'default';
    (row.querySelector('.gw-cert') as NlddTextField).value = 'some-tls-secret';

    expect(gatherGateways(form, 'inway', false)).toEqual([{ name: 'default' }]);
    expect(gatherGateways(form, 'inway', true)).toEqual([
      { name: 'default', certificate: { existingSecret: 'some-tls-secret' } },
    ]);
  });

  it('gives outways no self address field at all', () => {
    const row = addGatewayRow(form, 'outway');
    expect(row.querySelector('.gw-self')).toBeNull();
  });
});

describe('namespaceFieldHtml', () => {
  it('renders a dropdown of the host-provided namespaces', () => {
    const html = namespaceFieldHtml(['team-a', 'team-b']);
    expect(html).toContain('<nldd-dropdown>');
    expect(html).toContain('<option value="team-a">team-a</option>');
    expect(html).toContain('<option value="team-b">team-b</option>');
  });

  it('falls back to a text field when the host has no namespaces', () => {
    for (const empty of [undefined, []]) {
      const html = namespaceFieldHtml(empty);
      expect(html).toContain('<nldd-text-field id="namespace"');
      expect(html).not.toContain('<nldd-dropdown>');
    }
  });

  it('escapes namespace names rather than interpolating them raw', () => {
    const html = namespaceFieldHtml(['"><script>alert(1)</script>']);
    expect(html).not.toContain('<script>');
    expect(html).toContain('&lt;script&gt;');
  });

  it('is readable by trimmedValue in either shape', () => {
    const field = form.querySelector('#namespace-field') as HTMLElement;

    field.innerHTML = namespaceFieldHtml(['team-a']);
    expect(trimmedValue(form, 'namespace')).toBe('team-a');

    field.innerHTML = namespaceFieldHtml([]);
    (field.querySelector('#namespace') as unknown as { value: string }).value = 'typed-ns';
    expect(trimmedValue(form, 'namespace')).toBe('typed-ns');
  });
});
