import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  DestroyRef,
  effect,
  ElementRef,
  inject,
  signal,
  viewChild,
} from '@angular/core';
import TaskStore, { Task } from '../task-store';
import TaskStatusUi from '../task-status-ui.service';

interface DialogRef {
  nativeElement: { show?: () => void; hide?: () => void };
}

/**
 * The two questions a status menu can land on: what a task is waiting on, and
 * whether you are taking somebody else's over.
 *
 * Rendered wherever a status menu can be opened — the tasks page, and the sheet
 * the shell puts a task in. The state is not here but in TaskStatusUi, so the
 * two copies cannot disagree: whichever one is on screen shows the same
 * question about the same task.
 */
@Component({
  selector: 'app-task-status-dialogs',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './task-status-dialogs.html',
})
export default class TaskStatusDialogsComponent {
  protected readonly ui = inject(TaskStatusUi);

  protected readonly store = inject(TaskStore);

  private readonly waitingDialogEl = viewChild<ElementRef<HTMLElement>>('waitingDialogEl');

  /**
   * The ellipsis on an option that asks something further.
   *
   * "For Yara Nijhuis to start" is the whole answer, so it ends there. "For
   * someone to start" is not: a person still has to be pointed out, and the
   * three dots say the window is not done with you yet.
   */
  protected follow(task: Task): string {
    return this.ui.waitingPerson(task) === null ? '…' : '';
  }

  /**
   * Whether the missing person has been pointed out, which is only asked once
   * you have tried to save. A field that turns red before you have done anything
   * is telling you off for a form you have not filled in yet.
   */
  protected readonly missingWho = signal(false);

  protected onChoice(value: 'start' | 'finish' | 'other'): void {
    this.ui.waitingChoice.set(value);
    this.missingWho.set(false);
  }

  protected pickWho(id: string | null): void {
    this.ui.waitingWho.set(id);
    if (id !== null) this.missingWho.set(false);
  }

  /**
   * Save, or say what is missing.
   *
   * The button is never switched off: one that cannot be pressed does not say
   * why, and you are left guessing which of the fields it is waiting on.
   */
  protected save(event: Event, task: Task): void {
    event.preventDefault();
    if (!this.ui.canSaveWaiting(task)) {
      this.missingWho.set(true);
      return;
    }
    this.missingWho.set(false);
    this.ui.commitWaiting();
  }

  private readonly takeOverDialogEl = viewChild<ElementRef<HTMLElement>>('takeOverDialogEl');

  private readonly host = inject(ElementRef<HTMLElement>);

  constructor() {
    // The service decides when a dialog opens; it cannot reach into a template
    // to do it, so the template hands it the four handles. Registered under this
    // copy's own element, because there is more than one copy on screen and the
    // service picks the one the menu belongs to.
    effect(() => {
      const waiting = this.waitingDialogEl() as DialogRef | undefined;
      const takeOver = this.takeOverDialogEl() as DialogRef | undefined;
      this.ui.registerDialogs(this.host.nativeElement as HTMLElement, {
        showWaiting: () => waiting?.nativeElement.show?.(),
        hideWaiting: () => waiting?.nativeElement.hide?.(),
        showTakeOver: () => takeOver?.nativeElement.show?.(),
        hideTakeOver: () => takeOver?.nativeElement.hide?.(),
      });
    });

    inject(DestroyRef).onDestroy(() =>
      this.ui.unregisterDialogs(this.host.nativeElement as HTMLElement),
    );
  }
}
