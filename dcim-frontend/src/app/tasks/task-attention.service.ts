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

  private pending = false;

  readonly todayCount = computed(() => this.count());

  refresh(): void {
    if (this.pending) return;
    this.pending = true;
    const today = new Date().toISOString().slice(0, 10);
    firstValueFrom(this.taskApi.listTasks())
      .then((res) => {
        const tasks = res.tasks.map((t) => TaskApiService.mapTask(t));
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
      });
  }
}
