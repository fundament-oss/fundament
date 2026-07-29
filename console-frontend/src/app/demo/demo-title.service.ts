// Demo-only stand-in for TitleService. While the walkthrough is running the slide
// owns the document title, so page-level setTitle() calls from route components are
// dropped. Outside the walkthrough it is the real service, unchanged — hence the
// subclass: the title format lives in one place and cannot drift.
import { inject } from '@angular/core';
import { TitleService } from '../title.service';
import PresentationService from '../presentation/presentation.service';

export default class DemoTitleService extends TitleService {
  private presentation = inject(PresentationService);

  override setTitle(pageTitle?: string): void {
    if (this.presentation.active()) return;
    super.setTitle(pageTitle);
  }
}
