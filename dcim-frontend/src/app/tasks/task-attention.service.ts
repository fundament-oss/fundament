import { computed, inject, Injectable, signal } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import TaskApiService from '../task-management/task-api.service';
import connectErrorMessage from '../../connect/error';

/**
 * How many tasks are for today, kept where the shell can read it: the badge on
 * the Tasks section says what the day holds before you open it.
 *
 * The same rule as the Today view in the section's own menu, and it lives here
 * so the two cannot drift: due today or overdue, or urgent whatever the date,
 * and never anything that is already done.
 */
@Injectable({ providedIn: 'root' })
export default class TaskAttentionService {
  private readonly taskApi = inject(TaskApiService);

  private readonly count = signal(0);

  /** Every tag in use, so a form can offer the words already in the wild
   *  instead of asking everyone to type them again the same way. */
  private readonly allTags = signal<string[]>([]);

  private pending = false;

  private queued = false;

  readonly todayCount = computed(() => this.count());

  readonly tags = computed(() => this.allTags());

  /** Bumped whenever a task is made or changed somewhere else than the page
   *  showing the list, so that page can read the list again. */
  readonly changed = signal(0);

  /** Say that a task changed: the badge counts again and the list follows. */
  markChanged(): void {
    this.changed.update((n) => n + 1);
    this.refresh();
  }

  refresh(): void {
    if (this.pending) {
      // A refresh asked for while one is in flight cannot be dropped: the
      // answer on the wire was sent before whatever prompted this one, so it
      // does not contain it. Remember it and count once more when this lands.
      this.queued = true;
      return;
    }
    this.pending = true;
    const today = new Date().toISOString().slice(0, 10);
    firstValueFrom(this.taskApi.listTasks())
      .then((res) => {
        const tasks = res.tasks.map((t) => TaskApiService.mapTask(t));
        this.allTags.set([...new Set(tasks.flatMap((task) => task.tags))].sort());
        this.count.set(
          tasks.filter(
            (task) =>
              task.status !== 'Done' &&
              ((task.due !== '' && task.due <= today) || task.priority === 'Urgent'),
          ).length,
        );
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)))
      .finally(() => {
        this.pending = false;
        if (this.queued) {
          this.queued = false;
          this.refresh();
        }
      });
  }
}
