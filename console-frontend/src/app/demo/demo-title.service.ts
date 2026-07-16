// Demo-only stand-in for TitleService. While the walkthrough is running the slide
// owns the document title, so page-level setTitle() calls from route components are
// ignored. Outside the walkthrough it behaves like the real service.
import { inject } from '@angular/core';
import { Title, Meta } from '@angular/platform-browser';
import { PresentationService } from '../presentation/presentation.service';

export class DemoTitleService {
  private title = inject(Title);

  private meta = inject(Meta);

  private presentation = inject(PresentationService);

  private readonly DEFAULT_TITLE = 'Fundament Console';

  setTitle(pageTitle?: string): void {
    if (this.presentation.active()) return;
    this.title.setTitle(pageTitle ? `${pageTitle} - ${this.DEFAULT_TITLE}` : this.DEFAULT_TITLE);
  }

  setDescription(description: string): void {
    this.meta.updateTag({ name: 'description', content: description });
  }
}
