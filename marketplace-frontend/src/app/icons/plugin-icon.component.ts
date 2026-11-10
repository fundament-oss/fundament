import {
  CUSTOM_ELEMENTS_SCHEMA,
  ChangeDetectionStrategy,
  Component,
  computed,
  input,
} from '@angular/core';
import { isCustomIcon } from '../utils/custom-icon';

/**
 * Renders a plugin's logo as an `<img>` that keeps its brand colours, with an
 * optional dark variant. Plugins without a logo show the generic plugin icon.
 */
@Component({
  selector: 'app-plugin-icon',
  host: {
    class: 'contents',
  },
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <span
      [attr.class]="class()"
      [attr.role]="label() ? 'img' : null"
      [attr.aria-label]="label() || null"
      [attr.aria-hidden]="label() ? null : 'true'"
    >
      @if (lightLogo(); as light) {
        @if (darkLogo(); as dark) {
          <img class="h-full w-full object-contain dark:hidden" [src]="light" alt="" />
          <img class="hidden h-full w-full object-contain dark:block" [src]="dark" alt="" />
        } @else {
          <img
            class="h-full w-full rounded-sm object-contain dark:bg-gray-100"
            [src]="light"
            alt=""
          />
        }
      } @else {
        <nldd-icon name="plugin" color="accent"></nldd-icon>
      }
    </span>
  `,
})
export default class PluginIconComponent {
  label = input('');

  class = input('');

  logo = input('');

  logoDark = input('');

  protected lightLogo = computed(() => (isCustomIcon(this.logo()) ? this.logo() : ''));

  protected darkLogo = computed(() =>
    this.lightLogo() && isCustomIcon(this.logoDark()) ? this.logoDark() : '',
  );
}
