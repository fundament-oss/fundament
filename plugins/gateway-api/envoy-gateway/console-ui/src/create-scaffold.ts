// Shared wiring for every "create <resource>" view: loads the SDK + design
// system, renders the namespace control, restores Enter-to-submit, and drives the
// validate → build → k8s.create → navigate flow. Each view supplies only its
// resource ref, body builder, validation, and (optionally) extra field wiring.

import { loadSdk, loadNlddDesignSystem, navigateToDetail, navigateBack } from './shared.ts';
import { namespaceFieldHtml, trimmedValue } from './form-helpers.ts';
import type { NlddButton } from './nldd-design-system.ts';
import type { InitContext, K8sRef } from './sdk.ts';

export interface CreateFormOptions {
  // Text shown above the form once the SDK is ready.
  intro: string;
  // The resource to create (group/version/resource); namespace is added per submit.
  resource: Omit<K8sRef, 'namespace'>;
  // Builds the CR body from the form and namespace; must include metadata.name.
  buildBody: (form: ParentNode, namespace: string) => { metadata: { name: string } };
  // Returns false to block submit (missing/invalid required fields).
  validate: (form: ParentNode) => boolean;
  // Optional: wire extra toggles / dynamic UI after init (e.g. an HTTPS toggle).
  onReady?: (form: HTMLFormElement, ctx: InitContext) => void;
}

// An <nldd-dropdown> computes its label on `slotchange`, which can fire before
// programmatically-inserted <option>s exist. Re-dispatch it once rendered.
export function resyncDropdown(dropdown: HTMLElement | null): void {
  const apply = () =>
    dropdown?.shadowRoot?.querySelector('slot')?.dispatchEvent(new Event('slotchange'));
  (dropdown as (HTMLElement & { updateComplete?: Promise<unknown> }) | null)?.updateComplete?.then?.(apply);
  requestAnimationFrame(apply);
}

function renderNamespaceControl(namespaces: string[] | undefined): void {
  const field = document.getElementById('namespace-field') as HTMLElement;
  field.innerHTML = namespaceFieldHtml(namespaces);
  resyncDropdown(field.querySelector('nldd-dropdown'));
}

export async function mountCreateForm(opts: CreateFormOptions): Promise<void> {
  const intro = document.getElementById('intro') as HTMLElement;
  const form = document.getElementById('form') as HTMLFormElement;
  const errorBox = document.getElementById('error') as HTMLElement;
  const submitButton = document.getElementById('submit') as NlddButton;

  document.getElementById('back')!.addEventListener('click', () => navigateBack());

  let ctx: InitContext;
  try {
    await Promise.all([loadSdk(), loadNlddDesignSystem()]);
    ctx = await window.fundament.init;
  } catch (err) {
    intro.textContent = `Failed to load the plugin SDK: ${err instanceof Error ? err.message : err}`;
    return;
  }

  intro.textContent = opts.intro;
  renderNamespaceControl(ctx.namespaces);
  opts.onReady?.(form, ctx);
  form.hidden = false;

  // nldd-text-field's inner <input> is in shadow DOM and can't reach the light-DOM
  // form, so route Enter to the submit button to restore native Enter-to-submit.
  form.addEventListener('keydown', (e) => {
    const target = e.target as HTMLElement | null;
    if (e.key === 'Enter' && target?.tagName === 'NLDD-TEXT-FIELD') {
      e.preventDefault();
      submitButton.click();
    }
  });

  submitButton.addEventListener('click', async () => {
    // A programmatic .click() (Enter-to-submit) isn't blocked by `disabled`, so guard.
    if (submitButton.disabled) return;
    errorBox.hidden = true;
    // The namespace control is universal (dropdown for project routes, free-text
    // org-level). A programmatic .click() skips native `required`, so validate it
    // here — an empty namespace would otherwise build an invalid create request.
    const namespace = trimmedValue(form, 'namespace');
    if (!opts.validate(form) || !namespace) {
      errorBox.textContent = 'Please fill in the required fields.';
      errorBox.hidden = false;
      return;
    }

    submitButton.disabled = true;
    try {
      const body = opts.buildBody(form, namespace);
      const created = await window.fundament.k8s.create<{ metadata?: { name?: string } }>(
        { ...opts.resource, namespace },
        body,
      );
      navigateToDetail(created?.metadata?.name ?? body.metadata.name, namespace);
    } catch (err) {
      errorBox.textContent = `Failed to create: ${err instanceof Error ? err.message : err}`;
      errorBox.hidden = false;
      submitButton.disabled = false;
    }
  });
}
