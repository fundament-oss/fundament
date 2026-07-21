import { Component, inject, signal } from '@angular/core';
import { RouterOutlet } from '@angular/router';

import ThemeService from './theme.service';
import AppToast from './shared/toast';

@Component({
  selector: 'app-root',
  imports: [RouterOutlet, AppToast],
  templateUrl: './app.html',
})
export default class App {
  protected readonly title = signal('fundament-dcim');

  private readonly theme = inject(ThemeService);

  constructor() {
    // Apply the saved/system theme before any route renders.
    this.theme.initializeTheme();
  }
}
