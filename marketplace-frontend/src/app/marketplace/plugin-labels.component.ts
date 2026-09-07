import {
  Component,
  input,
  computed,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { type PluginLabel } from './marketplace.service';

interface LabelPresentation {
  text: string;
  color: string;
  icon: string;
}

// Colour and icon are deliberately distinct per label so a listing can be
// scanned without reading the tag text. Rijksoverheid keeps the lint blue of
// the ribbon it is named after.
const PRESENTATION: Record<PluginLabel, LabelPresentation> = {
  core: { text: 'Core', color: 'paars', icon: 'blocks-9' },
  rijksoverheid: { text: 'Rijksoverheid', color: 'lintblauw', icon: 'apartment-building' },
  'support-9-to-17': { text: '9-to-17 support', color: 'donkergroen', icon: 'clock' },
};

// Renders a plugin's trust/support labels as tags. Uses `display: contents` so
// the tags join the flex row of whichever container it is dropped into.
@Component({
  selector: 'app-plugin-labels',
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: { class: 'contents' },
  template: `
    @for (label of presentation(); track label.text) {
      <nldd-tag size="sm" [color]="label.color" [icon]="label.icon" [text]="label.text"></nldd-tag>
    }
  `,
})
export default class PluginLabelsComponent {
  labels = input.required<PluginLabel[]>();

  presentation = computed(() => this.labels().map((label) => PRESENTATION[label]));
}
