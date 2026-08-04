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

/**
 * Waits for a condition instead of for a duration: the custom elements render and
 * their MutationObservers flush on their own schedule, and a fixed sleep is
 * either slower than it needs to be or too short on a loaded CI runner.
 */
async function waitFor(condition: () => boolean, timeoutMs = 2000): Promise<boolean> {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    if (condition()) return true;
    if (Date.now() > deadline) return false;
    // eslint-disable-next-line no-await-in-loop
    await new Promise((resolve) => {
      setTimeout(resolve, 5);
    });
  }
}

/** Resolves once the elements are upgraded and have rendered once. */
async function upgraded(field: HTMLElement): Promise<void> {
  await customElements.whenDefined('nldd-form-field');
  await customElements.whenDefined('nldd-text-field');
  await (field as HTMLElement & { updateComplete?: Promise<unknown> }).updateComplete;
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

const shown = (error: HTMLElement) => () => error.hasAttribute('invalid');

describe('rewireFormFields', () => {
  it('nldd-form-field shows its error text while it stays in place', async () => {
    const { field, input, error } = buildField();
    await upgraded(field);

    markInvalid(input);

    expect(await waitFor(shown(error))).toBe(true);
  });

  it('restores error text visibility after the field is portalled', async () => {
    const { field, input, error } = buildField();
    await upgraded(field);

    document.body.appendChild(field);
    rewireFormFields(document.body);
    markInvalid(input);

    expect(await waitFor(shown(error))).toBe(true);
  });

  it('error text stays hidden while the input is valid', async () => {
    const { field, error } = buildField();
    await upgraded(field);

    document.body.appendChild(field);
    rewireFormFields(document.body);

    // Short window on purpose: nothing should ever flip this on, so the only
    // question is whether the rewire itself does by accident.
    expect(await waitFor(shown(error), 100)).toBe(false);
  });
});
