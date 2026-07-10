import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';

/**
 * Renders a plugin's SVG logo (from /img/plugins/<name>.svg) as a CSS mask so it
 * can be tinted with `text-*`/`bg-current`, since a plain `<img>` can't inherit
 * page color the way an inline `fill="currentColor"` SVG can.
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

  protected maskStyle = computed(() => {
    const url = `url(/img/plugins/${this.name()}.svg)`;
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
