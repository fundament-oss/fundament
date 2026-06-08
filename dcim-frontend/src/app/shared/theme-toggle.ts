import { ChangeDetectionStrategy, Component, CUSTOM_ELEMENTS_SCHEMA, inject } from '@angular/core';

import ThemeService from '../theme.service';

// Sun/moon segmented control for switching between light and dark mode.
// Reused in the shell header and the standalone technician view.
@Component({
  selector: 'app-theme-toggle',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  template: `
    <nldd-segmented-control
      [value]="theme.isDarkMode() ? 'dark' : 'light'"
      variant="icon"
      size="sm"
      (change)="theme.setTheme($any($event).detail.value)"
    >
      <nldd-segmented-control-item
        value="light"
        text="Light mode"
        icon="sun"
      ></nldd-segmented-control-item>
      <nldd-segmented-control-item
        value="dark"
        text="Dark mode"
        icon="moon"
      ></nldd-segmented-control-item>
    </nldd-segmented-control>
  `,
})
export default class ThemeToggleComponent {
  protected readonly theme = inject(ThemeService);
}
