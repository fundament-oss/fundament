import { ChangeDetectionStrategy, Component, CUSTOM_ELEMENTS_SCHEMA, input } from '@angular/core';

import '@nldd/design-system/badge';
import '@nldd/design-system/container';
import '@nldd/design-system/popover';
import '@nldd/design-system/rich-text';

let nextId = 0;

/**
 * The "Mock" badge, and what it means. A bare nldd-badge is never interactive,
 * so it can say "Mock" but cannot explain it: this wraps it in a button that
 * opens a popover with the explanation (a bottom sheet on phones). Click rather
 * than hover, so it works the same with a mouse, a keyboard and on touch.
 */
@Component({
  selector: 'app-mock-badge',
  host: {
    class: 'contents',
  },
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  template: `
    <!-- A native popovertarget lets the browser own toggle, light dismiss and
         Escape. -->
    <button
      type="button"
      [id]="triggerId"
      [attr.popovertarget]="popoverId"
      aria-label="Mock"
      class="focus-visible:outline-accent-500 dark:focus-visible:outline-accent-400 inline-flex cursor-pointer rounded-full border-0 bg-transparent p-0 align-middle focus-visible:outline-2 focus-visible:outline-offset-2"
    >
      <nldd-badge color="hemelblauw" size="sm" text="Mock" decorative></nldd-badge>
    </button>
    <nldd-popover
      [id]="popoverId"
      [attr.anchor]="triggerId"
      role="region"
      placement="bottom-start"
      accessible-label="What Mock means"
    >
      <nldd-container padding="16">
        <nldd-rich-text spacing="flat">
          <p>
            <strong>{{ label() }}</strong>
          </p>
          <p>{{ explanation() }}</p>
        </nldd-rich-text>
      </nldd-container>
    </nldd-popover>
  `,
})
export default class MockBadgeComponent {
  /** Short and specific to the place: the popover's lead. */
  label = input.required<string>();

  /** The rest of the popover, for places with something more specific to say. */
  explanation = input(
    'This part of the console is not connected to the platform yet. What you see here is example data, and changes are not saved or applied.',
  );

  protected readonly triggerId = `mock-badge-${(nextId += 1)}`;

  protected readonly popoverId = `${this.triggerId}-popover`;
}
