import { quietRestoredTooltip, rewireFormFields } from './sheet-sync.directive';

// happy-dom has no ElementInternals, which the design system's form-associated mixin needs in its
// constructor. A stub is enough: these tests only exercise validation-item visibility.
(HTMLElement.prototype as unknown as { attachInternals: () => unknown }).attachInternals = () => ({
  setFormValue() {},
  setValidity() {},
  form: null,
});

await import('@nldd/design-system/form-field');
await import('@nldd/design-system/text-field');
await import('@nldd/design-system/validation-list');

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
  await customElements.whenDefined('nldd-validation-list');
  await (field as HTMLElement & { updateComplete?: Promise<unknown> }).updateComplete;
}

function buildField(): { field: HTMLElement; input: HTMLElement; error: HTMLElement } {
  const host = document.createElement('div');
  host.innerHTML = `
    <nldd-form-field label="Namespace name">
      <nldd-text-field unmet="namespace-name-error"></nldd-text-field>
      <nldd-validation-list>
        <nldd-validation-item id="namespace-name-error">Namespace name is required.</nldd-validation-item>
      </nldd-validation-list>
    </nldd-form-field>`;
  document.body.appendChild(host);
  return {
    field: host.querySelector('nldd-form-field') as HTMLElement,
    input: host.querySelector('nldd-text-field') as HTMLElement,
    error: host.querySelector('nldd-validation-item') as HTMLElement,
  };
}

function markInvalid(input: HTMLElement): void {
  input.setAttribute('invalid', '');
}

const shown = (item: HTMLElement) => () => item.hasAttribute('unmet');

describe('rewireFormFields', () => {
  it('nldd-form-field shows its validation item while it stays in place', async () => {
    const { field, input, error } = buildField();
    await upgraded(field);

    markInvalid(input);

    expect(await waitFor(shown(error))).toBe(true);
  });

  it('restores validation item visibility after the field is portalled', async () => {
    const { field, input, error } = buildField();
    await upgraded(field);

    document.body.appendChild(field);
    rewireFormFields(document.body);
    markInvalid(input);

    expect(await waitFor(shown(error))).toBe(true);
  });

  it('validation item stays hidden while the input is valid', async () => {
    const { field, error } = buildField();
    await upgraded(field);

    document.body.appendChild(field);
    rewireFormFields(document.body);

    // Short window on purpose: nothing should ever flip this on, so the only
    // question is whether the rewire itself does by accident.
    expect(await waitFor(shown(error), 100)).toBe(false);
  });
});

describe('quietRestoredTooltip', () => {
  const setup = () => {
    const sheet = document.createElement('nldd-sheet');
    const button = document.createElement('nldd-icon-button');
    const inner = document.createElement('button');
    button.appendChild(inner);
    document.body.append(sheet, button);
    return { sheet, button, inner };
  };

  const ancestors = (node: HTMLElement): HTMLElement[] =>
    node.parentElement ? [node.parentElement, ...ancestors(node.parentElement)] : [];

  const focusIn = (target: HTMLElement): FocusEvent => {
    const event = new FocusEvent('focusin', { bubbles: true, composed: true });
    Object.defineProperty(event, 'composedPath', { value: () => [target, ...ancestors(target)] });
    return event;
  };

  afterEach(() => {
    document.body.replaceChildren();
  });

  it('holds back the tooltip of the button focus returns to after a pointer, until focus leaves', () => {
    const { sheet, button, inner } = setup();
    document.dispatchEvent(new PointerEvent('pointerdown'));
    quietRestoredTooltip(focusIn(inner), sheet);
    expect(button.getAttribute('tooltip-timing')).toBe('never');
    button.dispatchEvent(new FocusEvent('focusout'));
    expect(button.hasAttribute('tooltip-timing')).toBe(false);
  });

  it('leaves the tooltip after the keyboard', () => {
    const { sheet, button, inner } = setup();
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    quietRestoredTooltip(focusIn(inner), sheet);
    expect(button.hasAttribute('tooltip-timing')).toBe(false);
  });

  it('leaves a button inside the sheet, and a timing the markup set', () => {
    const { sheet, button, inner } = setup();
    document.dispatchEvent(new PointerEvent('pointerdown'));
    const own = document.createElement('nldd-icon-button');
    sheet.appendChild(own);
    quietRestoredTooltip(focusIn(own), sheet);
    expect(own.hasAttribute('tooltip-timing')).toBe(false);

    button.setAttribute('tooltip-timing', 'instant');
    quietRestoredTooltip(focusIn(inner), sheet);
    expect(button.getAttribute('tooltip-timing')).toBe('instant');
  });
});
