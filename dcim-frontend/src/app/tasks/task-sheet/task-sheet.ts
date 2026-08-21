import {
  ChangeDetectionStrategy,
  Component,
  computed,
  CUSTOM_ELEMENTS_SCHEMA,
  effect,
  inject,
  viewChild,
} from '@angular/core';
import OverlayService from '../../shell/overlay.service';
import TaskFormComponent from '../task-form/task-form';

interface NativeElementRef {
  nativeElement: { show?: () => void; hide?: () => void };
}

/**
 * The sheet that holds the task form, in the shell rather than on the tasks
 * page.
 *
 * Writing down what has to happen is the thing you do while looking at
 * something else: a rack that is full, an asset that is broken. So the form
 * opens from the add button in the bar over whatever page you are on, and the
 * page it belongs to does not have to be there. Editing a task you already
 * have open takes the same form, but as a second view inside that sheet: see
 * the tasks page.
 */
@Component({
  selector: 'app-task-sheet',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  imports: [TaskFormComponent],
  templateUrl: './task-sheet.html',
})
export default class TaskSheetComponent {
  protected readonly overlays = inject(OverlayService);

  protected readonly form = this.overlays.taskSheet;

  protected readonly title = computed(() => (this.form()?.id ? 'Edit task' : 'New task'));

  private readonly sheetEl = viewChild<NativeElementRef>('taskSheet');

  constructor() {
    effect(() => {
      const el = this.sheetEl()?.nativeElement;
      if (this.form() !== null) el?.show?.();
      else el?.hide?.();
    });
  }

  protected close(): void {
    this.overlays.closeTask();
  }
}
