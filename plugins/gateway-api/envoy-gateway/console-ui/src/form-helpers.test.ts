import { describe, expect, it } from 'vitest';
import { fieldError, validateFields } from './form-helpers.ts';

// The NLDD elements are not registered under happy-dom, so `value` is set as a
// plain property, the way the real component exposes it.
function field(
  attrs: string,
  value: string,
): HTMLElement & { value?: string } {
  document.body.innerHTML = `<nldd-text-field ${attrs}></nldd-text-field>`;
  const el = document.body.firstElementChild as HTMLElement & {
    value?: string;
  };
  el.value = value;
  return el;
}

describe('fieldError', () => {
  it('requires a value only when the field is required', () => {
    expect(fieldError(field('required', '  '))).toBe('This field is required.');
    expect(fieldError(field('', ''))).toBeNull();
  });

  it('checks maxlength', () => {
    expect(fieldError(field('maxlength="3"', 'abcd'))).toBe(
      'Use at most 3 characters.',
    );
  });

  it('anchors the pattern and prefers data-error as the message', () => {
    expect(fieldError(field('pattern="[a-z]+"', 'abc1'))).toBe('');
    expect(
      fieldError(field('pattern="[a-z]+" data-error="Letters only."', 'abc1')),
    ).toBe('Letters only.');
    expect(fieldError(field('pattern="[a-z]+"', 'abc'))).toBeNull();
  });

  it('checks a number against min and max', () => {
    const attrs = 'type="number" min="1" max="65535"';
    expect(fieldError(field(attrs, '1.5'))).toBe('Enter a whole number.');
    expect(fieldError(field(attrs, '70000'))).toBe(
      'Enter a number from 1 to 65535.',
    );
    expect(fieldError(field(attrs, '443'))).toBeNull();
  });

  it('names only the bound a number has', () => {
    expect(fieldError(field('type="number" min="1"', '0'))).toBe(
      'Enter a number of at least 1.',
    );
    expect(fieldError(field('type="number" max="10"', '11'))).toBe(
      'Enter a number of at most 10.',
    );
  });
});

describe('validateFields', () => {
  function renderForm(): HTMLFormElement {
    document.body.innerHTML = `
      <form id="form">
        <nldd-text-field id="name" required unmet="name-error"></nldd-text-field>
        <nldd-validation-item id="name-error">Enter a valid name.</nldd-validation-item>
        <div hidden>
          <nldd-text-field id="secret" required></nldd-text-field>
        </div>
      </form>`;
    return document.getElementById('form') as HTMLFormElement;
  }

  it('marks a failing field and writes the message into its item', () => {
    const form = renderForm();
    expect(validateFields(form)).toBe(false);
    expect(form.querySelector('#name')!.hasAttribute('invalid')).toBe(true);
    expect(document.getElementById('name-error')!.textContent).toBe(
      'This field is required.',
    );
  });

  it('skips fields inside a hidden section', () => {
    const form = renderForm();
    (form.querySelector('#name') as HTMLElement & { value: string }).value =
      'web';
    expect(validateFields(form)).toBe(true);
    expect(form.querySelector('#secret')!.hasAttribute('invalid')).toBe(false);
  });
});
