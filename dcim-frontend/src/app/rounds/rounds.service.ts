import { computed, inject, Injectable, signal } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import TaskApiService, { TaskData } from '../task-management/task-api.service';
import TaskStepApiService from '../task-management/task-step-api.service';
import UserApiService, { RosterUser } from '../task-management/user-api.service';
import DatacenterListService from '../datacenters/datacenter-list.service';
import settledPool from '../shared/settled-pool';
import connectErrorMessage from '../../connect/error';
import { buildRounds, Round, roundsByDay, unplacedTasks } from './round';

/** Ceiling on in-flight ListTaskSteps calls, as the walkthrough and the admin
 *  board's bulk actions use. */
const STEP_FETCH_CONCURRENCY = 6;

/**
 * Every round there is, derived from the tasks rather than stored.
 *
 * Held in a service instead of in the page because the tasks page wants a piece
 * of it too: the box in a task's detail says whose round that task sits in.
 */
@Injectable({ providedIn: 'root' })
export default class RoundsService {
  private readonly taskApi = inject(TaskApiService);

  private readonly stepApi = inject(TaskStepApiService);

  private readonly userApi = inject(UserApiService);

  private readonly datacenterList = inject(DatacenterListService);

  private readonly tasks = signal<TaskData[]>([]);

  private readonly people = signal<RosterUser[]>([]);

  /** Steps done and steps in total, per task. What a round has covered is the
   *  sum over its tasks, so the sidebar and the walkthrough count the same
   *  thing. */
  private readonly stepCounts = signal<Record<string, { done: number; total: number }>>({});

  readonly loaded = signal(false);

  readonly loadError = signal<string | null>(null);

  private pending = false;

  /** The same rule as the Today view in Tasks, so the two agree. */
  readonly todayISO = (): string => new Date().toISOString().slice(0, 10);

  private readonly datacenterNames = computed(() =>
    this.datacenterList.datacenters().map((dc) => dc.name),
  );

  readonly rounds = computed(() =>
    buildRounds(
      this.tasks(),
      (id) => this.people().find((p) => p.id === id)?.name ?? 'Unknown',
      this.datacenterNames(),
      this.todayISO(),
    ),
  );

  readonly days = computed(() => roundsByDay(this.rounds()));

  readonly unplaced = computed(() => unplacedTasks(this.tasks(), this.datacenterNames()));

  /** The round a given task sits in, for the box in its detail. */
  readonly roundOf = computed(() => {
    const map = new Map<string, Round>();
    this.rounds().forEach((round) => round.tasks.forEach((task) => map.set(task.id, round)));
    return map;
  });

  isToday(day: string): boolean {
    return day === this.todayISO();
  }

  /** How far a round has come, counted in steps. A task without steps counts as
   *  one step, the same as it walks. */
  progress(round: Round): { done: number; total: number } {
    const counts = this.stepCounts();
    return round.tasks.reduce(
      (sum, task) => {
        const count = counts[task.id] ?? { done: 0, total: 1 };
        return { done: sum.done + count.done, total: sum.total + Math.max(1, count.total) };
      },
      { done: 0, total: 0 },
    );
  }

  /**
   * How far a round has come, counted in tasks. That is the unit somebody asks
   * a round about: three of five done says more about a trip than sixteen of
   * forty steps, which counts the inside of the work rather than the work.
   */
  taskProgress(round: Round): { done: number; total: number } {
    const counts = this.stepCounts();
    const done = round.tasks.filter((task) => {
      const c = counts[task.id];
      return !!c && c.total > 0 && c.done === c.total;
    }).length;
    return { done, total: round.tasks.length };
  }

  find(personId: string, datacenter: string, day: string): Round | undefined {
    return this.rounds().find(
      (r) => r.personId === personId && r.datacenter === datacenter && r.day === day,
    );
  }

  /** Reads everything again while keeping what is on screen, so opening the
   *  section shows the rounds it had and fills them in underneath. */
  load(): void {
    if (this.pending) return;
    this.pending = true;
    this.datacenterList.load();
    this.read().finally(() => {
      this.pending = false;
      this.loaded.set(true);
    });
  }

  private async read(): Promise<void> {
    try {
      const [taskRes, userRes] = await Promise.all([
        firstValueFrom(this.taskApi.listTasks()),
        firstValueFrom(this.userApi.listUsers()),
      ]);
      this.people.set(userRes.users.map((u) => UserApiService.mapUser(u)));
      const tasks = taskRes.tasks.map((t) => TaskApiService.mapTask(t));
      this.tasks.set(tasks);
      this.loadError.set(null);
      await this.readSteps(tasks.filter((t) => t.status !== 'Done'));
    } catch (err) {
      const message = connectErrorMessage(err);
      // eslint-disable-next-line no-console
      console.error(message);
      this.loadError.set(message);
    }
  }

  /**
   * One ListTaskSteps call per open task, through a bounded pool. A partial
   * answer is fine here where it is not in the walkthrough: a count that is
   * short reads as less progress, and the next load corrects it, where a
   * walkthrough with gaps in it would walk somebody past their own work.
   */
  private async readSteps(tasks: TaskData[]): Promise<void> {
    const settled = await settledPool(tasks, STEP_FETCH_CONCURRENCY, async (task) => {
      const res = await firstValueFrom(this.stepApi.listTaskSteps(task.id));
      return {
        id: task.id,
        done: res.steps.filter((s) => s.completed).length,
        total: res.steps.length,
      };
    });
    const counts: Record<string, { done: number; total: number }> = {};
    settled.forEach((r) => {
      if (r.status === 'fulfilled')
        counts[r.value.id] = { done: r.value.done, total: r.value.total };
    });
    this.stepCounts.set(counts);
  }
}
