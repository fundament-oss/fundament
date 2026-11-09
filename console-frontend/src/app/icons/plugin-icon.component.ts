import {
  CUSTOM_ELEMENTS_SCHEMA,
  ChangeDetectionStrategy,
  Component,
  PLATFORM_ID,
  computed,
  effect,
  inject,
  input,
  signal,
} from '@angular/core';
import { PlatformLocation, isPlatformBrowser } from '@angular/common';
import '@nldd/design-system/icon';
import { isCustomIcon } from '../utils/custom-icon';

/**
 * Renders a plugin's logo. A published logo is an `<img>` that keeps its brand
 * colours (with an optional dark variant). Without one, the built-in SVG
 * (<base href>img/plugins/<name>.svg) is a CSS mask tinted with `bg-current`;
 * if that file is missing, the generic plugin icon is shown.
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
      } @else if (builtInMissing()) {
        <nldd-icon name="plugin" color="accent"></nldd-icon>
      } @else {
        <span
          class="text-accent-500 dark:text-accent-400 block h-full w-full bg-current"
          [style]="maskStyle()"
        ></span>
      }
    </span>
  `,
})
export default class PluginIconComponent {
  name = input.required<string>();

  label = input('');

  class = input('');

  logo = input('');

  logoDark = input('');

  protected lightLogo = computed(() => (isCustomIcon(this.logo()) ? this.logo() : ''));

  protected darkLogo = computed(() =>
    this.lightLogo() && isCustomIcon(this.logoDark()) ? this.logoDark() : '',
  );

  // A failed CSS mask fires no error event, so the built-in file is probed with an Image.
  protected builtInMissing = signal(false);

  // Assets live under the app's base href, which the demo build moves to
  // /marketplace/ so the bundle can be served from inside the console demo. A
  // root-absolute /img/plugins/... would leave that subtree and silently pick up
  // whatever the host app happens to serve there. Read from the DOM rather than
  // through Location.prepareExternalUrl(), which the demo's hash routing would
  // turn into a fragment.
  private readonly baseHref = inject(PlatformLocation).getBaseHrefFromDOM().replace(/\/+$/, '');

  private readonly isBrowser = isPlatformBrowser(inject(PLATFORM_ID));

  private readonly builtInUrl = computed(() => `${this.baseHref}/img/plugins/${this.name()}.svg`);

  constructor() {
    effect((onCleanup) => {
      const url = this.builtInUrl();
      this.builtInMissing.set(false);
      if (!this.isBrowser || this.lightLogo()) return;
      const probe = new Image();
      probe.onerror = () => this.builtInMissing.set(true);
      onCleanup(() => {
        probe.onerror = null;
      });
      probe.src = url;
    });
  }

  protected maskStyle = computed(() => {
    const url = `url(${this.builtInUrl()})`;
    return {
      'mask-image': url,
      '-webkit-mask-image': url,
      'mask-repeat': 'no-repeat',
      '-webkit-mask-repeat': 'no-repeat',
      'mask-position': 'center',
      '-webkit-mask-position': 'center',
      'mask-size': 'contain',
      '-webkit-mask-size': 'contain',
    };
  });
}
