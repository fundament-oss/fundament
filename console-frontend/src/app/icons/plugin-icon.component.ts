import { ChangeDetectionStrategy, Component, computed, inject, input } from '@angular/core';
import { PlatformLocation } from '@angular/common';

/**
 * Renders a plugin's SVG logo (from <base href>img/plugins/<name>.svg) as a CSS
 * mask so it can be tinted with `text-*`/`bg-current`, since a plain `<img>`
 * can't inherit page color the way an inline `fill="currentColor"` SVG can.
 */
@Component({
  selector: 'app-plugin-icon',
  host: {
    class: 'contents',
  },
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <span
      [attr.class]="class()"
      [attr.role]="label() ? 'img' : null"
      [attr.aria-label]="label() || null"
      [attr.aria-hidden]="label() ? null : 'true'"
    >
      <span
        class="text-accent-500 dark:text-accent-400 block h-full w-full bg-current"
        [style]="maskStyle()"
      ></span>
    </span>
  `,
})
export default class PluginIconComponent {
  name = input.required<string>();

  label = input('');

  class = input('');

  // Assets live under the app's base href, which the demo build moves to
  // /marketplace/ so the bundle can be served from inside the console demo. A
  // root-absolute /img/plugins/... would leave that subtree and silently pick up
  // whatever the host app happens to serve there. Read from the DOM rather than
  // through Location.prepareExternalUrl(), which the demo's hash routing would
  // turn into a fragment.
  private readonly baseHref = inject(PlatformLocation).getBaseHrefFromDOM().replace(/\/+$/, '');

  protected maskStyle = computed(() => {
    const url = `url(${this.baseHref}/img/plugins/${this.name()}.svg)`;
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
