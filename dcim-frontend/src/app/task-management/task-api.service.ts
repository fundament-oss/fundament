import { Injectable, inject } from '@angular/core';
import { timestampDate, timestampFromDate } from '@bufbuild/protobuf/wkt';
import {
  TaskStatus as ProtoStatus,
  TaskPriority as ProtoPriority,
} from '../../generated/v1/task_pb';
import type { Task as ProtoTask } from '../../generated/v1/task_pb';
import { TASK_CLIENT } from '../../connect/tokens';

// How far the work has got. Whose turn it is follows from the assignee, and
// being stuck on something that is not a person is blockedReason.
export type TaskStatusLabel = 'To do' | 'Doing' | 'Done';
// 'None' is what a task carries until somebody prioritizes it, and it reads as
// an empty circle in a picker and as nothing at all on the task itself.
export type TaskPriorityLabel = 'Urgent' | 'High' | 'Medium' | 'Low' | 'None';

/** The admin/board view-model of a task (display strings, no proto enums). */
export interface TaskData {
  id: string;
  title: string;
  description: string;
  status: TaskStatusLabel;
  priority: TaskPriorityLabel;
  tags: string[];
  /** Stuck on something that is not a person; null when the work can move. */
  blockedReason: string | null;
  location: string;
  assignee: string | null;
  due: string;
  created: string;
}

/** Fields needed to create a task. */
export interface TaskInput {
  title: string;
  description: string;
  status: TaskStatusLabel;
  priority: TaskPriorityLabel;
  tags: string[];
  location: string;
  assignee: string | null;
  due: string;
  /** What the work is stuck on, when it is not a person. `null` means it is
   *  not stuck; an empty string means stuck without saying on what. */
  blockedReason: string | null;
}

/**
 * The fields an update actually touches. Only the keys present are sent, so a
 * status-only change (kanban drag, bulk action) cannot overwrite a field another
 * admin edited in the meantime. Within a present key, an empty value clears the
 * column: `assignee: null`, `due: ''`, `location: ''` and `description: ''` all
 * mean "remove".
 */
export type TaskPatch = Partial<TaskInput>;

/**
 * A due date is a calendar date, but the column is timestamptz. Both directions
 * therefore go through UTC midnight — parsing as local midnight instead would
 * shift the stored instant into the previous day for every UTC+ timezone, and
 * each subsequent save would shift it again.
 */
const dueToDate = (due: string): Date => new Date(`${due}T00:00:00Z`);

/** The epoch is the "empty" sentinel that clears due_date server-side. */
const CLEAR_DUE_DATE = new Date(0);

/**
 * Reached only when the server sends an enum value this build has no label for
 * — a schema that moved ahead of the frontend. The display falls back so the
 * board still renders, but the mismatch is logged rather than passed off as a
 * legitimate value.
 */
function unknownEnum<T>(kind: string, value: never, fallback: T): T {
  // eslint-disable-next-line no-console
  console.warn(`Unknown Task${kind} from API: ${String(value)} — displaying "${String(fallback)}"`);
  return fallback;
}

@Injectable({ providedIn: 'root' })
export default class TaskApiService {
  private readonly client = inject(TASK_CLIENT);

  /**
   * The keys of `input` whose value differs from the task as the board last
   * loaded it. Every TaskInput field is also a TaskData field, and both use the
   * same "empty" spellings ('' / null), so a plain !== comparison is enough —
   * an untouched field never lands in the patch, and so is never written back
   * over an edit someone else made in the meantime.
   */
  static changedFields(current: TaskData, input: TaskInput): TaskPatch {
    const patch: TaskPatch = {};
    (Object.keys(input) as (keyof TaskInput)[])
      .filter((key) => input[key] !== current[key])
      .forEach((key) => Object.assign(patch, { [key]: input[key] }));
    return patch;
  }

  /** Maps an API task onto the admin/board view-model. */
  static mapTask(t: ProtoTask): TaskData {
    return {
      id: t.id,
      title: t.title,
      description: t.description,
      status: TaskApiService.fromProtoStatus(t.status),
      priority: TaskApiService.fromProtoPriority(t.priority),
      tags: [...t.tags],
      blockedReason: t.blockedReason || null,
      location: t.location,
      assignee: t.assigneeId ? t.assigneeId : null,
      due: t.dueDate ? timestampDate(t.dueDate).toISOString().slice(0, 10) : '',
      created: t.created ? timestampDate(t.created).toISOString().slice(0, 10) : '',
    };
  }

  static fromProtoStatus(s: ProtoStatus): TaskStatusLabel {
    switch (s) {
      case ProtoStatus.TODO:
        return 'To do';
      case ProtoStatus.DOING:
        return 'Doing';
      case ProtoStatus.DONE:
        return 'Done';
      case ProtoStatus.UNSPECIFIED:
        return 'To do';
      default:
        return unknownEnum('Status', s, 'To do');
    }
  }

  private static toProtoStatus(s: TaskStatusLabel): ProtoStatus {
    const map: Record<TaskStatusLabel, ProtoStatus> = {
      'To do': ProtoStatus.TODO,
      Doing: ProtoStatus.DOING,
      Done: ProtoStatus.DONE,
    };
    return map[s];
  }

  static fromProtoPriority(p: ProtoPriority): TaskPriorityLabel {
    switch (p) {
      case ProtoPriority.LOW:
        return 'Low';
      case ProtoPriority.MEDIUM:
        return 'Medium';
      case ProtoPriority.HIGH:
        return 'High';
      case ProtoPriority.URGENT:
        return 'Urgent';
      case ProtoPriority.UNSPECIFIED:
        return 'None';
      default:
        return unknownEnum('Priority', p, 'None');
    }
  }

  private static toProtoPriority(p: TaskPriorityLabel): ProtoPriority {
    const map: Record<TaskPriorityLabel, ProtoPriority> = {
      None: ProtoPriority.UNSPECIFIED,
      Low: ProtoPriority.LOW,
      Medium: ProtoPriority.MEDIUM,
      High: ProtoPriority.HIGH,
      Urgent: ProtoPriority.URGENT,
    };
    return map[p];
  }

  listTasks(assigneeId?: string) {
    return this.client.listTasks(assigneeId ? { assigneeId } : {});
  }

  getTask(id: string) {
    return this.client.getTask({ id });
  }

  /**
   * The nullable columns are omitted when the form left them blank, so they are
   * created NULL rather than as an empty string. Sending `''` would give the
   * table two spellings of "no location": `''` on tasks that never had one, NULL
   * on tasks where it was cleared later through updateTask.
   */
  createTask(input: TaskInput) {
    return this.client.createTask({
      title: input.title,
      status: TaskApiService.toProtoStatus(input.status),
      priority: TaskApiService.toProtoPriority(input.priority),
      tags: input.tags,
      ...(input.description ? { description: input.description } : {}),
      ...(input.location ? { location: input.location } : {}),
      ...(input.assignee ? { assigneeId: input.assignee } : {}),
      ...(input.due ? { dueDate: timestampFromDate(dueToDate(input.due)) } : {}),
    });
  }

  /**
   * Sends only the fields the caller put in `patch`; anything absent is left
   * untouched server-side. Pass the full form for an edit, or a single key for
   * a drag or bulk action.
   */
  updateTask(id: string, patch: TaskPatch) {
    return this.client.updateTask({
      id,
      ...('title' in patch ? { title: patch.title } : {}),
      ...('status' in patch ? { status: TaskApiService.toProtoStatus(patch.status!) } : {}),
      ...('priority' in patch ? { priority: TaskApiService.toProtoPriority(patch.priority!) } : {}),
      ...('tags' in patch ? { tags: patch.tags! } : {}),
      // The empty value of a present field clears the column: the backend maps
      // an empty string / the epoch onto a NULL write.
      ...('description' in patch ? { description: patch.description ?? '' } : {}),
      ...('assignee' in patch ? { assigneeId: patch.assignee ?? '' } : {}),
      ...('location' in patch ? { location: patch.location ?? '' } : {}),
      // Blocked is the one field an empty value cannot clear: "" is a task that
      // is stuck without saying on what, which is not the same as one that is
      // not stuck. Hence the flag the API carries beside it.
      ...(patch.blockedReason === null ? { clearBlockedReason: true } : {}),
      ...(typeof patch.blockedReason === 'string' ? { blockedReason: patch.blockedReason } : {}),
      ...('due' in patch
        ? { dueDate: timestampFromDate(patch.due ? dueToDate(patch.due) : CLEAR_DUE_DATE) }
        : {}),
    });
  }

  deleteTask(id: string) {
    return this.client.deleteTask({ id });
  }
}
