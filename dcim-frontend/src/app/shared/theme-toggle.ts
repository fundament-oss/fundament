import { ChangeDetectionStrategy, Component, CUSTOM_ELEMENTS_SCHEMA, inject } from '@angular/core';
import ThemeService from '../theme.service';

@Component({
  selector: 'app-theme-toggle',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  template: `
    <nldd-segmented-control
      [value]="theme.isDark() ? 'dark' : 'light'"
      variant="icon"
      size="sm"
      (change)="onChange($event)"
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

  protected onChange(event: Event): void {
    const value = (event as CustomEvent<{ value: string }>).detail.value;
    this.theme.set(value === 'dark' ? 'dark' : 'light');
  }
}
