import { rewireFormFields } from './sheet-sync.directive';

// happy-dom has no ElementInternals, which the design system's form-associated mixin needs in its
// constructor. A stub is enough: these tests only exercise error-text visibility.
(HTMLElement.prototype as unknown as { attachInternals: () => unknown }).attachInternals = () => ({
  setFormValue() {},
  setValidity() {},
  form: null,
});

await import('@nldd/design-system/form-field');
await import('@nldd/design-system/text-field');

/** Lets the custom elements render and their MutationObservers flush. */
function settle(): Promise<void> {
  return new Promise((resolve) => {
    setTimeout(resolve, 20);
  });
}

function buildField(): { field: HTMLElement; input: HTMLElement; error: HTMLElement } {
  const host = document.createElement('div');
  host.innerHTML = `
    <nldd-form-field label="Namespace name">
      <nldd-text-field error-message="namespace-name-error"></nldd-text-field>
      <nldd-form-field-error-text id="namespace-name-error">
        Namespace name is required.
      </nldd-form-field-error-text>
    </nldd-form-field>`;
  document.body.appendChild(host);
  return {
    field: host.querySelector('nldd-form-field') as HTMLElement,
    input: host.querySelector('nldd-text-field') as HTMLElement,
    error: host.querySelector('nldd-form-field-error-text') as HTMLElement,
  };
}

function markInvalid(input: HTMLElement): void {
  input.setAttribute('invalid', '');
}

describe('rewireFormFields', () => {
  it('nldd-form-field shows its error text while it stays in place', async () => {
    const { input, error } = buildField();
    await settle();

    markInvalid(input);
    await settle();

    expect(error.hasAttribute('invalid')).toBe(true);
  });

  it('restores error text visibility after the field is portalled', async () => {
    const { field, input, error } = buildField();
    await settle();

    document.body.appendChild(field);
    rewireFormFields(document.body);
    await settle();

    markInvalid(input);
    await settle();

    expect(error.hasAttribute('invalid')).toBe(true);
  });

  it('error text stays hidden while the input is valid', async () => {
    const { field, error } = buildField();
    await settle();

    document.body.appendChild(field);
    rewireFormFields(document.body);
    await settle();

    expect(error.hasAttribute('invalid')).toBe(false);
  });
});
