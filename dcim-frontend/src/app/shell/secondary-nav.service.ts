import { Injectable, TemplateRef, signal } from '@angular/core';

/**
 * The second sidebar is owned by the shell but filled by the page: a rack list,
 * a set of categories, a column of filters. The page knows that content and the
 * data behind it, so it hands over a template rather than the shell reaching
 * into every screen for it.
 *
 * A page registers in ngOnInit and clears on destroy, so leaving a section
 * takes its menu with it.
 */
@Injectable({ providedIn: 'root' })
export default class SecondaryNavService {
  private readonly template = signal<TemplateRef<unknown> | null>(null);

  /** What the shell renders in the second sidebar, or nothing. */
  readonly content = this.template.asReadonly();

  set(template: TemplateRef<unknown>): void {
    this.template.set(template);
  }

  /** Only clears what this page put there, so a page that leaves after the next
   *  one has already registered cannot wipe the new menu. */
  clear(template: TemplateRef<unknown>): void {
    if (this.template() === template) this.template.set(null);
  }
}
