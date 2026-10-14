import { CUSTOM_ELEMENTS_SCHEMA, Component, inject, signal } from '@angular/core';
import { RouterOutlet } from '@angular/router';

import { ConfigService } from './config.service';
import ThemeService from './theme.service';
import ToastComponent from './shared/toast';

@Component({
  selector: 'app-root',
  imports: [RouterOutlet, ToastComponent],
  templateUrl: './app.html',
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
})
export default class App {
  protected readonly title = signal('fundament-dcim');

  private readonly theme = inject(ThemeService);

  /** The same site on the PoC; set only in a test environment, which the
   *  banner above the app then says this is. */
  protected readonly pocUrl = inject(ConfigService).getConfig().pocUrl ?? '';

  constructor() {
    // Apply the saved/system theme before any route renders.
    this.theme.initializeTheme();
  }
}
