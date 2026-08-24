import { computed, inject, Injectable, signal } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import DatacenterListService from '../datacenters/datacenter-list.service';
import TaskAttentionService from '../tasks/task-attention.service';
import ToastService from '../shared/toast.service';
import PlacementApiService, { RackOption } from '../inventory/placement-api.service';
import connectErrorMessage from '../../connect/error';
import { taskTags } from '../tasks/task-tags';
import TaskApiService, {
  TaskData,
  TaskInput,
  TaskPatch,
  TaskPriorityLabel,
  TaskStatusLabel,
} from './task-api.service';
import UserApiService, { RosterUser } from './user-api.service';

export type Technician = RosterUser;

export interface Note {
  author: string;
  // The author's roster id, or null for an unattributed note (written by someone
  // outside the directory). Kept alongside the display name so the avatar can be
  // resolved by id rather than by matching names.
  authorId: string | null;
  text: string;
  time: string;
}

/** A task with whatever of its notes has been fetched. */
export interface Task extends TaskData {
  notes: Note[];
}

/**
 * The tasks, who they are for, and every write to them.
 *
 * It lives here rather than on the tasks page because a task is edited from two
 * places: the page, and the sheet the shell opens over whatever you were
 * looking at. Two copies of the list would answer differently the moment one of
 * them wrote, so there is one.
 *
 * What is here is what does not depend on being on a page: the data, the writes,
 * and the readings that need the roster to make sense — who has a task, and
 * therefore whether it is yours to move. The page keeps its filters, its
 * selection and its menus.
 */
@Injectable({ providedIn: 'root' })
export default class TaskStore {
  private readonly taskApi = inject(TaskApiService);

  private readonly userApi = inject(UserApiService);

  private readonly placementApi = inject(PlacementApiService);

  private readonly datacenterList = inject(DatacenterListService);

  private readonly attention = inject(TaskAttentionService);

  private readonly toast = inject(ToastService);

  readonly tasks = signal<Task[]>([]);

  readonly technicians = signal<Technician[]>([]);

  readonly currentUser = signal<Technician | null>(null);

  readonly racks = signal<RackOption[]>([]);

  readonly loadError = signal<string | null>(null);

  private readonly dateLocale = 'en-US';

  /** The three a task can be in. Waiting is derived, so it is not one of them. */
  readonly statuses: TaskStatusLabel[] = ['To do', 'Doing', 'Done'];

  // — Reading ——————————————————————————————————————————————————————————————

  getTech(id: string | null): Technician | null {
    return this.technicians().find((t) => t.id === id) ?? null;
  }

  /** The name the combo box shows for whoever it is for. */
  assigneeName(task: TaskData): string {
    if (!task.assignee) return '';
    return this.technicians().find((t) => t.id === task.assignee)?.name ?? '';
  }

  /**
   * Whether a task is yours. An unknown current user is not the same fact as
   * "assigned to somebody else": the answer is unavailable, so nothing is
   * filtered out on it and you see a full list rather than a convincing empty
   * one.
   */
  isMine(task: TaskData): boolean {
    const me = this.currentUser()?.id;
    if (!me) return true;
    return task.assignee === me;
  }

  /** Whose task this is, if not yours. Nobody, or you, both read as yours. */
  somebodyElses(task: TaskData): boolean {
    const me = this.currentUser()?.id;
    return !!task.assignee && task.assignee !== me && !!this.getTech(task.assignee);
  }

  /**
   * Why a task is not yours to move. Being stuck on something that is not a
   * person is stored on the task; lying with a person is not stored at all but
   * read off the assignee, because it depends on who is looking: the task you
   * are waiting for is the task somebody else has to do. Returns nothing when
   * the work is yours, or finished.
   */
  holdReason(task: TaskData): string | null {
    if (task.status === 'Done') return null;

    // What only a person can know: stuck on a thing rather than on somebody.
    // Printed as it was written, not built up here: this one is a whole sentence
    // in whatever language it was typed, where the two below are the app's own
    // words about the roster and can be translated with the rest of the app.
    if (task.blockedReason !== null) {
      return task.blockedReason || 'Waiting';
    }

    if (!task.assignee) return null;

    const me = this.currentUser()?.id;
    if (me && task.assignee === me) return null;

    const who = this.getTech(task.assignee);
    if (!who) return null;

    // Derived rather than written down: who has it is the assignee and how far
    // they are is the status, so this sentence cannot go stale when either
    // changes. Writing it by hand would repeat both and then drift from them.
    return task.status === 'Doing'
      ? `Waiting on ${who.name} to finish`
      : `Waiting on ${who.name} to start`;
  }

  /**
   * What is not yours to move. Somebody else's to-do is your waiting-for, and a
   * task stuck on a part rather than a person waits just as hard. Same rule as
   * the line under the title, so the view and the row can never disagree.
   */
  isWaiting(task: TaskData): boolean {
    return this.holdReason(task) !== null;
  }

  readonly priorityIcon = (priority: TaskPriorityLabel): string => {
    const map: Record<TaskPriorityLabel, string> = {
      Urgent: 'high-priority-filled',
      High: 'high-priority',
      Medium: 'medium-priority',
      Low: 'low-priority',
      None: 'no-priority',
    };
    return map[priority];
  };

  readonly statusIcon = (status: TaskStatusLabel): string => {
    const map: Record<TaskStatusLabel, string> = {
      'To do': 'to-do',
      Doing: 'doing',
      Done: 'done',
    };
    return map[status];
  };

  readonly taskStateIcon = (task: TaskData): string =>
    (this.holdReason(task) !== null ? 'clock-light' : `${this.statusIcon(task.status)}-light`);

  /** Everything a task is filed under, as one list of paths. See task-tags.ts. */
  readonly taskTagList = (task: TaskData): string[] => taskTags(task);

  // Uses the trailing (random) hex of the uuid rather than the leading bytes,
  // which in uuidv7 are a millisecond timestamp and collide across tasks
  // created close together.
  readonly taskDisplayId = (task: Task): string =>
    // A task that does not exist yet has no number to show. The row stays, so
    // the sheet does not change shape the moment it gets one.
    (task.id ? `T-${task.id.replace(/-/g, '').slice(-8).toUpperCase()}` : '—');

  formatDate(str: string | null): string {
    if (!str) return '—';
    const d = new Date(`${str}T00:00:00`);
    return d.toLocaleDateString(this.dateLocale, {
      month: 'short',
      day: 'numeric',
      year: 'numeric',
    });
  }

  /** Every tag on offer: the data centers, their racks, and whatever else is
   *  already in use somewhere. */
  readonly knownTags = computed(() => {
    const sites = this.datacenterList.datacenters().map((dc) => dc.name);
    const racks = this.racks().map((rack) => `${rack.datacenter}/${rack.name}`);
    const fixed = [...sites, ...racks];
    const used = this.attention.tags().filter((tag) => !fixed.includes(tag));
    return [...fixed, ...used];
  });

  // — Loading ———————————————————————————————————————————————————————————————

  loadTasks(): void {
    firstValueFrom(this.taskApi.listTasks())
      .then((res) => {
        this.tasks.update((previous) => {
          const notesById = new Map(previous.map((t) => [t.id, t.notes]));
          return res.tasks.map((t) => ({
            ...TaskApiService.mapTask(t),
            notes: notesById.get(t.id) ?? [],
          }));
        });
        this.loadError.set(null);
      })
      .catch((err) => {
        const message = connectErrorMessage(err);
        // eslint-disable-next-line no-console
        console.error(message);
        this.loadError.set(message);
        this.toast.error('Could not load tasks');
      });
  }

  loadRoster(): void {
    firstValueFrom(this.userApi.listUsers())
      .then((res) => {
        this.technicians.set(res.users.map((u) => UserApiService.mapUser(u)));
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  loadRacks(): void {
    if (this.racks().length > 0) return;
    this.placementApi
      .listRackOptions()
      .then((racks) => this.racks.set(racks))
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  // — Writing ———————————————————————————————————————————————————————————————

  /**
   * Writes one field of one task and nothing else.
   *
   * Changing a task is a series of small facts, each true the moment you set
   * it, so there is nothing to submit: the field you touched is the patch.
   * Sending the whole snapshot instead would write back every field as this
   * board last saw it and silently revert what somebody else changed.
   *
   * The new value is on screen before the write comes back, because that is
   * where you just put it. A failed write puts the old one back and says which
   * field it was, in a message that stays: by then the sheet it belonged to may
   * be closed, and one that fades takes the loss with it.
   */
  patchTask(task: TaskData, patch: TaskPatch, what: string): void {
    const before: Partial<TaskData> = {};
    (Object.keys(patch) as (keyof TaskPatch)[]).forEach((key) => {
      Object.assign(before, { [key]: task[key] });
    });
    this.tasks.update((list) => list.map((t) => (t.id === task.id ? { ...t, ...patch } : t)));

    firstValueFrom(this.taskApi.updateTask(task.id, patch))
      .then(() => this.loadTasks())
      .catch((err) => {
        // eslint-disable-next-line no-console
        console.error(connectErrorMessage(err));
        this.tasks.update((list) => list.map((t) => (t.id === task.id ? { ...t, ...before } : t)));
        this.toast.error(`Could not save ${what} — it has been put back`);
      });
  }

  /**
   * Writes a task that did not exist yet, and answers with its id.
   *
   * There is no submit here either: this runs the moment the first field of a
   * new task is set, and everything after it is an ordinary patch. Rejecting
   * rather than toasting, because what the caller does next depends on it — it
   * has a half-made task on screen.
   */
  async createTask(values: TaskInput): Promise<string> {
    const res = await firstValueFrom(this.taskApi.createTask(values));
    // Awaited, not fired off: the caller switches to the new id the moment this
    // resolves, and a list that does not hold it yet would answer "no such task"
    // for a frame — long enough to see the sheet blink empty.
    await this.reloadTasks();
    return res.taskId;
  }

  private async reloadTasks(): Promise<void> {
    try {
      const res = await firstValueFrom(this.taskApi.listTasks());
      this.tasks.update((previous) => {
        const notesById = new Map(previous.map((t) => [t.id, t.notes]));
        return res.tasks.map((t) => ({
          ...TaskApiService.mapTask(t),
          notes: notesById.get(t.id) ?? [],
        }));
      });
      this.loadError.set(null);
    } catch (err) {
      const message = connectErrorMessage(err);
      // eslint-disable-next-line no-console
      console.error(message);
      this.loadError.set(message);
    }
  }
}
