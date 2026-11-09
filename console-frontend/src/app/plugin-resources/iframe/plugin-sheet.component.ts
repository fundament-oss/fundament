import {
  afterNextRender,
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  computed,
  inject,
  input,
  output,
  signal,
} from '@angular/core';
import SheetSyncDirective from '../../sheet-sync.directive';
import PluginRegistryService from '../plugin-registry.service';
import { kindToSingularLabel, labelMidSentence } from '../crd-schema.utils';
import type { OpenSheet } from './sheet-request';

import '@nldd/design-system/page';
import '@nldd/design-system/sheet';
import '@nldd/design-system/simple-section';
import '@nldd/design-system/top-title-bar';

/**
 * The console's own sheet around a plugin's create, edit or detail page, over
 * the plugin page that asked for it (the page itself is projected in: see
 * plugin-iframe.component.ts). The sheet is the console's so that its
 * backdrop covers the whole window and not only the plugin's frame, and so
 * the title, the plugin it comes from and the way out are the console's to
 * say. What runs inside is still a plugin page in the same sandbox as any
 * other.
 */
@Component({
  selector: 'app-plugin-sheet',
  imports: [SheetSyncDirective],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <nldd-sheet
      appSheetSync
      [show]="open()"
      placement="right"
      width="560px"
      [attr.accessible-label]="title()"
      (close)="onClosed()"
    >
      <nldd-page sticky-header>
        <nldd-top-title-bar
          slot="header"
          [attr.text]="title()"
          [attr.supporting-text]="source()"
          dismiss-text="Close"
          (dismiss)="close(false)"
        ></nldd-top-title-bar>
        <nldd-simple-section>
          <ng-content />
        </nldd-simple-section>
      </nldd-page>
    </nldd-sheet>
  `,
})
export default class PluginSheetComponent {
  private registry = inject(PluginRegistryService);

  request = input.required<OpenSheet>();

  /** The installation name, as in the plugin's routes. */
  pluginName = input.required<string>();

  /** The sheet is gone. True when the page in it saved something. */
  closed = output<boolean>();

  protected open = signal(true);

  /** True once SheetSyncDirective has moved the sheet to document.body. The
   *  page in it waits for this (see plugin-iframe.component.ts): an iframe
   *  that moves reloads, so a page put in before the move loads twice. */
  readonly placed = signal(false);

  private changed = false;

  constructor() {
    afterNextRender(() => this.placed.set(true));
  }

  /** Written here, not taken from the plugin: a plugin naming its own sheet
   *  could title it "Sign in again". */
  protected title = computed(() => {
    const { kind, view, resource } = this.request();
    switch (view) {
      case 'create':
        return `New ${labelMidSentence(kindToSingularLabel(kind))}`;
      case 'edit':
        return `Edit ${resource?.name ?? ''}`;
      case 'detail':
        return resource?.name ?? '';
      default: {
        const exhaustive: never = view;
        throw new Error(`Unhandled sheet view: ${String(exhaustive)}`);
      }
    }
  });

  /** Which plugin this is, so what it shows is never read as the console's. */
  protected source = computed(() => {
    const label = this.registry.getPlugin(this.pluginName())?.label;
    return label ? `From the ${label} plugin` : 'From a plugin';
  });

  /** Closes the sheet with its animation. The output follows from the sheet's
   *  own close event, which Escape and a backdrop click send too. */
  close(changed: boolean): void {
    this.changed ||= changed;
    this.open.set(false);
  }

  protected onClosed(): void {
    this.closed.emit(this.changed);
  }
}
