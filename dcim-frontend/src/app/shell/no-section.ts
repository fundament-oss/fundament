import { ChangeDetectionStrategy, Component, CUSTOM_ELEMENTS_SCHEMA } from '@angular/core';

/**
 * The app with no section chosen.
 *
 * It needs an address of its own. Narrowed to two columns this is where back
 * from a section's menu lands, and without a route the shell would have to hold
 * that step as state beside the url. The two then disagree: the outlet keeps
 * rendering the page you left, and widening the window lays out a depth the
 * address never knew about.
 *
 * Side by side there is a third column with nothing in it, so it says so, the
 * same words in the same place as a section with nothing picked.
 */
@Component({
  selector: 'app-no-section',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  template: `
    <nldd-page>
      <nldd-simple-section width="full" height="40dvh" vertical-alignment="center">
        <nldd-inline-dialog text="No selection"></nldd-inline-dialog>
      </nldd-simple-section>
    </nldd-page>
  `,
})
export default class NoSection {}
