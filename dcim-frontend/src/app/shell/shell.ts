import { ChangeDetectionStrategy, Component, CUSTOM_ELEMENTS_SCHEMA } from '@angular/core';
import { RouterLink, RouterLinkActive, RouterOutlet } from '@angular/router';

import ThemeToggleComponent from '../shared/theme-toggle';

// Shell wraps routes that share the nav header; task-management-technician sits outside it, since it has a different layout.
@Component({
  selector: 'app-shell',
  templateUrl: './shell.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink, RouterLinkActive, RouterOutlet, ThemeToggleComponent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
})
export default class ShellComponent {}
