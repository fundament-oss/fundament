import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  effect,
  inject,
  viewChild,
} from '@angular/core';
import OverlayService from '../../shell/overlay.service';
import TaskDetailComponent from '../task-detail/task-detail';

interface NativeElementRef {
  nativeElement: { show?: () => void; hide?: () => void };
}

/**
 * The sheet that holds the task form, in the shell rather than on the tasks
 * page.
 *
 * Writing down what has to happen is the thing you do while looking at
 * something else: a rack that is full, an asset that is broken. So it opens
 * from the add button in the bar over whatever page you are on, and the page it
 * belongs to does not have to be there.
 *
 * What is inside is the same app-task-detail the tasks page shows, on a task
 * that does not exist yet. There is no submit: the first field you set is what
 * makes the task real, and closing without setting one leaves nothing behind.
 */
@Component({
  selector: 'app-task-sheet',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  imports: [TaskDetailComponent],
  templateUrl: './task-sheet.html',
})
export default class TaskSheetComponent {
  protected readonly overlays = inject(OverlayService);

  protected readonly form = this.overlays.taskSheet;

  private readonly sheetEl = viewChild<NativeElementRef>('taskSheet');

  private readonly detail = viewChild(TaskDetailComponent);

  constructor() {
    effect(() => {
      const el = this.sheetEl()?.nativeElement;
      if (this.form() === null) {
        el?.hide?.();
        return;
      }
      // A blank task every time it opens: what you typed and abandoned last time
      // is not a draft you meant to keep.
      this.detail()?.startDraft();
      el?.show?.();
    });
  }

  protected close(): void {
    this.overlays.closeTask();
  }
}
