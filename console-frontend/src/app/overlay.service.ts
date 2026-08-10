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

  /** The create flows from the toolbar. They belong to no page, so opening one
   *  from there must not send you to a page first: the sheet floats over
   *  whatever you were reading. Their addresses still exist, and a guard sends
   *  you to the page the thing lands on with the sheet already open. */
  readonly newCluster = signal(false);

  readonly inviteMember = signal(false);

  /** Project-scoped, so these hold the project id rather than a flag. */
  readonly newNamespace = signal<string | null>(null);

  readonly addProjectMember = signal<string | null>(null);

  /** A sheet the shell owns stands over the page you opened it from. Go
   *  somewhere else and it has nothing left to stand on, so it goes with you.
   *  The guards that open one from an address do so after their redirect has
   *  landed, or this would close what they just opened. */
  closeAll(): void {
    this.profile.set(false);
    this.apiKeys.set(false);
    this.newProject.set(false);
    this.newCluster.set(false);
    this.inviteMember.set(false);
    this.newNamespace.set(null);
    this.addProjectMember.set(null);
  }
}

export default OverlayService;
