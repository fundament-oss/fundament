import {
  CUSTOM_ELEMENTS_SCHEMA,
  ChangeDetectionStrategy,
  Component,
  computed,
  input,
} from '@angular/core';
import '@nldd/design-system/icon-cell';
import { isCustomIcon } from '../utils/custom-icon';

const FALLBACK_ICON = 'circle-dashed';
const INVALID_MARKERS = ['/', '.svg', '.png', 'sha256:'];

/**
 * A plugin menu icon: an NLDD icon name, or a data: URI drawn as a CSS mask so it
 * follows the text colour. Anything else (URLs, digests) falls back to a placeholder.
 */
@Component({
  selector: 'app-plugin-menu-icon',
  host: {
    class: 'contents',
  },
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @if (customUrl()) {
      <nldd-icon-cell size="20">
        <span class="block h-5 w-5 bg-current" [style]="maskStyle()"></span>
      </nldd-icon-cell>
    } @else {
      <nldd-icon-cell [attr.icon]="iconName()" size="20"></nldd-icon-cell>
    }
  `,
})
export default class PluginMenuIconComponent {
  icon = input<string | undefined>();

  protected customUrl = computed(() => {
    const value = this.icon();
    return value && isCustomIcon(value) ? value : null;
  });

  protected iconName = computed(() => {
    const value = this.icon();
    if (!value || INVALID_MARKERS.some((marker) => value.includes(marker))) return FALLBACK_ICON;
    return value;
  });

  protected maskStyle = computed(() => {
    const url = `url("${this.customUrl()}")`;
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
