import { Directive, ElementRef, effect, inject, input } from '@angular/core';

type DialogElement = HTMLElement & { show(): void; hide(): void };

@Directive({
  selector: 'nldd-modal-dialog[appDialogSync]',
})
export default class DialogSyncDirective {
  private el = inject<ElementRef<DialogElement>>(ElementRef);

  show = input(false);

  private prev = false;

  private sync = effect(() => {
    const show = this.show();
    if (show && !this.prev) {
      this.el.nativeElement.show();
    } else if (!show && this.prev) {
      this.el.nativeElement.hide();
    }
    this.prev = show;
  });
}
