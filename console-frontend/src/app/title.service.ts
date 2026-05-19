import { Injectable, inject } from '@angular/core';
import { Title, Meta } from '@angular/platform-browser';

@Injectable({
  providedIn: 'root',
})
// eslint-disable-next-line import-x/prefer-default-export
export class TitleService {
  private title = inject(Title);

  private meta = inject(Meta);

  private readonly DEFAULT_TITLE = 'Fundament Console';

  private readonly SUFFIX = ' — Fundament Console';

  setTitle(pageTitle?: string): void {
    if (!pageTitle) {
      this.title.setTitle(this.DEFAULT_TITLE);
    } else {
      this.title.setTitle(pageTitle + this.SUFFIX);
    }
  }

  setDescription(description: string): void {
    this.meta.updateTag({ name: 'description', content: description });
  }
}
