export default function focusFirstModalInput(dialog: HTMLElement): void {
  const candidates = Array.from(
    dialog.querySelectorAll<HTMLElement>(
      'input:not([disabled]), select:not([disabled]), nldd-text-field:not([disabled]), nldd-search-field:not([disabled]), nldd-button:not([disabled]), button:not([disabled])',
    ),
  );
  const el =
    candidates.find((candidate) => !candidate.closest('[slot="actions"]')) ??
    candidates.find((candidate) => !!candidate.closest('[slot="actions"]'));
  if (!el) return;
  const inner =
    el.shadowRoot?.querySelector<HTMLElement>('button:not([disabled]), input:not([disabled])') ??
    el;
  inner.focus({ focusVisible: true } as FocusOptions);
}
