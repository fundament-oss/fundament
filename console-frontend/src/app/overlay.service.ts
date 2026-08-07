import { Injectable, signal } from '@angular/core';

/**
 * The sheets the shell owns rather than the router: your account, your keys and
 * a new project. They open over whatever page you are on, so routing to them
 * would unmount that page and leave an empty pane behind the sheet.
 *
 * The addresses still exist. A guard on each one flips the matching signal and
 * sends you home, so a link or a bookmark opens the sheet over the home pane
 * instead of a page that is not there.
 */
@Injectable({ providedIn: 'root' })
export class OverlayService {
  readonly profile = signal(false);

  readonly apiKeys = signal(false);

  readonly newProject = signal(false);
}

export default OverlayService;
