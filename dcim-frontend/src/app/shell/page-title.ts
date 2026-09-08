import { Injectable, inject } from '@angular/core';
import { Title } from '@angular/platform-browser';
import { RouterStateSnapshot, TitleStrategy } from '@angular/router';

/** What every tab ends with, so a window full of them still says which app. */
const APP_NAME = 'Fundament DCIM';

/**
 * The specific first, the general last: a browser full of tabs is read by its
 * first words, so those have to be the ones that tell two tabs apart. The
 * middle dot is the separator the design guidelines name.
 */
export function pageTitle(specific: string): string {
  return specific ? `${specific} · ${APP_NAME}` : APP_NAME;
}

/**
 * Turns the `title` of a route into the tab's title. A page that knows
 * something more specific than its route does (the name of a rack, of an
 * asset) sets it again with `pageTitle()` once that lands, which is after this
 * has run.
 */
@Injectable()
export default class AppTitleStrategy extends TitleStrategy {
  private readonly title = inject(Title);

  override updateTitle(snapshot: RouterStateSnapshot): void {
    this.title.setTitle(pageTitle(this.buildTitle(snapshot) ?? ''));
  }
}
