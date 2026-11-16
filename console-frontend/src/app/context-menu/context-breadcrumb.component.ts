import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  computed,
  inject,
  input,
  output,
} from '@angular/core';
import '@nldd/design-system/button';
import '@nldd/design-system/icon-button';
import NavContextService, { CONTEXT_KINDS } from '../nav-context.service';
import PageNavService from '../page-nav.service';

/**
 * The top of the menu, in two parts: an arrow up to the place this one sits in,
 * and the name of the place you are in. The name is a button rather than a
 * title, because what you do with where you are is go somewhere else: it opens
 * the picker with the whole organization in it.
 *
 * Only the name, not the path to it: the arrow already says there is something
 * above, and in a narrow sidebar a full trail would truncate the one part that
 * matters. The icon says what kind of place it is.
 *
 * The pane needs a heading of its own (see app.html), and the button cannot be
 * one, so it is there for assistive technology only and says the kind as well.
 */
@Component({
  selector: 'app-context-breadcrumb',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  template: `
    <h1 class="sr-only">{{ kindLabel() }} {{ current().name }}</h1>
    <div class="flex items-center gap-1">
      @if (parent(); as parent) {
        <nldd-icon-button
          icon="arrow-left"
          variant="neutral-transparent"
          [attr.href]="pageNav.path(parent.path)"
          [attr.accessible-label]="'Back to ' + parent.name"
          (click)="pageNav.navigate($event, parent.path)"
        ></nldd-icon-button>
      }
      <!-- Bordered and as wide as its name, like a select: it reads as a
           control, and the chevron sits at its edge rather than floating in
           the middle of the row. A long name truncates within the pane. -->
      <nldd-button
        class="min-w-0"
        type="button"
        variant="neutral-base"
        max-width="100%"
        single-line
        popup-type="dialog"
        end-icon="chevron-up-chevron-down"
        [attr.start-icon]="kindIcon()"
        [attr.text]="current().name"
        [attr.accessible-label]="kindLabel() + ' ' + current().name + ', switch'"
        [attr.expanded]="pickerOpen() ? '' : null"
        (click)="openPicker.emit()"
      ></nldd-button>
    </div>
  `,
})
export default class ContextBreadcrumbComponent {
  protected pageNav = inject(PageNavService);

  private navContext = inject(NavContextService);

  /** Whether the picker this opens is on screen, for `aria-expanded`. */
  readonly pickerOpen = input(false);

  readonly openPicker = output<void>();

  protected current = this.navContext.current;

  protected parent = this.navContext.parent;

  protected kindLabel = computed(() => CONTEXT_KINDS[this.current().kind].label);

  protected kindIcon = computed(() => CONTEXT_KINDS[this.current().kind].icon);
}
