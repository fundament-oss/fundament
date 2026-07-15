import { Injectable, inject } from '@angular/core';
import { timestampDate, timestampFromDate } from '@bufbuild/protobuf/wkt';
import {
  TaskStatus as ProtoStatus,
  TaskPriority as ProtoPriority,
  TaskCategory as ProtoCategory,
} from '../../generated/v1/task_pb';
import type { Task as ProtoTask } from '../../generated/v1/task_pb';
import { TASK_CLIENT } from '../../connect/tokens';

export type TaskStatusLabel = 'Ready' | 'In Progress' | 'Review' | 'Blocked' | 'Done';
export type TaskPriorityLabel = 'Critical' | 'High' | 'Medium' | 'Low';
export type TaskCategoryLabel = 'Hardware' | 'Network' | 'Cooling' | 'Power' | 'Security' | 'Other';

/** The admin/board view-model of a task (display strings, no proto enums). */
export interface TaskData {
  id: string;
  title: string;
  description: string;
  status: TaskStatusLabel;
  priority: TaskPriorityLabel;
  category: TaskCategoryLabel;
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
  category: TaskCategoryLabel;
  location: string;
  assignee: string | null;
  due: string;
}

/**
 * The fields an update actually touches. Only the keys present are sent, so a
 * status-only change (kanban drag, bulk action) cannot overwrite a field another
 * admin edited in the meantime. Within a present key, an empty value clears the
 * column: `assignee: null`, `due: ''` and `location: ''` all mean "remove".
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

@Injectable({ providedIn: 'root' })
export default class TaskApiService {
  private readonly client = inject(TASK_CLIENT);

  /** Maps an API task onto the admin/board view-model. */
  static mapTask(t: ProtoTask): TaskData {
    return {
      id: t.id,
      title: t.title,
      description: t.description,
      status: TaskApiService.fromProtoStatus(t.status),
      priority: TaskApiService.fromProtoPriority(t.priority),
      category: TaskApiService.fromProtoCategory(t.category),
      location: t.location,
      assignee: t.assigneeId ? t.assigneeId : null,
      due: t.dueDate ? timestampDate(t.dueDate).toISOString().slice(0, 10) : '',
      created: t.created ? timestampDate(t.created).toISOString().slice(0, 10) : '',
    };
  }

  static fromProtoStatus(s: ProtoStatus): TaskStatusLabel {
    switch (s) {
      case ProtoStatus.READY:
        return 'Ready';
      case ProtoStatus.IN_PROGRESS:
        return 'In Progress';
      case ProtoStatus.REVIEW:
        return 'Review';
      case ProtoStatus.BLOCKED:
        return 'Blocked';
      case ProtoStatus.DONE:
        return 'Done';
      default:
        return 'Ready';
    }
  }

  private static toProtoStatus(s: TaskStatusLabel): ProtoStatus {
    const map: Record<TaskStatusLabel, ProtoStatus> = {
      Ready: ProtoStatus.READY,
      'In Progress': ProtoStatus.IN_PROGRESS,
      Review: ProtoStatus.REVIEW,
      Blocked: ProtoStatus.BLOCKED,
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
      case ProtoPriority.CRITICAL:
        return 'Critical';
      default:
        return 'Medium';
    }
  }

  private static toProtoPriority(p: TaskPriorityLabel): ProtoPriority {
    const map: Record<TaskPriorityLabel, ProtoPriority> = {
      Low: ProtoPriority.LOW,
      Medium: ProtoPriority.MEDIUM,
      High: ProtoPriority.HIGH,
      Critical: ProtoPriority.CRITICAL,
    };
    return map[p];
  }

  static fromProtoCategory(c: ProtoCategory): TaskCategoryLabel {
    switch (c) {
      case ProtoCategory.HARDWARE:
        return 'Hardware';
      case ProtoCategory.NETWORK:
        return 'Network';
      case ProtoCategory.COOLING:
        return 'Cooling';
      case ProtoCategory.POWER:
        return 'Power';
      case ProtoCategory.SECURITY:
        return 'Security';
      case ProtoCategory.OTHER:
        return 'Other';
      default:
        return 'Other';
    }
  }

  private static toProtoCategory(c: TaskCategoryLabel): ProtoCategory {
    const map: Record<TaskCategoryLabel, ProtoCategory> = {
      Hardware: ProtoCategory.HARDWARE,
      Network: ProtoCategory.NETWORK,
      Cooling: ProtoCategory.COOLING,
      Power: ProtoCategory.POWER,
      Security: ProtoCategory.SECURITY,
      Other: ProtoCategory.OTHER,
    };
    return map[c];
  }

  listTasks(assigneeId?: string) {
    return this.client.listTasks(assigneeId ? { assigneeId } : {});
  }

  getTask(id: string) {
    return this.client.getTask({ id });
  }

  createTask(input: TaskInput) {
    return this.client.createTask({
      title: input.title,
      description: input.description,
      status: TaskApiService.toProtoStatus(input.status),
      priority: TaskApiService.toProtoPriority(input.priority),
      category: TaskApiService.toProtoCategory(input.category),
      location: input.location,
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
      ...('description' in patch ? { description: patch.description } : {}),
      ...('status' in patch ? { status: TaskApiService.toProtoStatus(patch.status!) } : {}),
      ...('priority' in patch ? { priority: TaskApiService.toProtoPriority(patch.priority!) } : {}),
      ...('category' in patch ? { category: TaskApiService.toProtoCategory(patch.category!) } : {}),
      // The empty value of a present field clears the column: the backend maps
      // an empty string / the epoch onto a NULL write.
      ...('assignee' in patch ? { assigneeId: patch.assignee ?? '' } : {}),
      ...('location' in patch ? { location: patch.location ?? '' } : {}),
      ...('due' in patch
        ? { dueDate: timestampFromDate(patch.due ? dueToDate(patch.due) : CLEAR_DUE_DATE) }
        : {}),
    });
  }

  deleteTask(id: string) {
    return this.client.deleteTask({ id });
  }
}
