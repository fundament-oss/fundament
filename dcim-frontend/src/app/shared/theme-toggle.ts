import { ChangeDetectionStrategy, Component, CUSTOM_ELEMENTS_SCHEMA, inject } from '@angular/core';
import ThemeService from '../theme.service';

@Component({
  selector: 'app-theme-toggle',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  template: `
    <!-- Three, like the service and the menu in the shell: following the
         operating system is a setting of its own, not the absence of a choice.
         Bound to the preference rather than to what is on screen, or picking
         System while the system is dark would light up Dark. -->
    <nldd-segmented-control
      [value]="theme.themePreference()"
      variant="icon"
      (change)="theme.setTheme($any($event).detail.value)"
    >
      <nldd-segmented-control-item
        value="system"
        text="System"
        icon="display"
      ></nldd-segmented-control-item>
      <nldd-segmented-control-item
        value="light"
        text="Light mode"
        icon="light-mode"
      ></nldd-segmented-control-item>
      <nldd-segmented-control-item
        value="dark"
        text="Dark mode"
        icon="dark-mode"
      ></nldd-segmented-control-item>
    </nldd-segmented-control>
  `,
})
export default class ThemeToggleComponent {
  protected readonly theme = inject(ThemeService);
}
