import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import TaskApiService, { TaskData, TaskInput } from './task-api.service';
import {
  TaskSchema,
  TaskStatus as ProtoStatus,
  TaskPriority as ProtoPriority,
  TaskCategory as ProtoCategory,
} from '../../generated/v1/task_pb';

const baseTask: TaskData = {
  id: '019dce10-0000-7000-8000-000000000001',
  title: 'Replace broken harddisk',
  description: 'Failed disk in Bay 3',
  status: 'In Progress',
  priority: 'Critical',
  category: 'Hardware',
  location: 'Rack 123',
  assignee: '019dce30-0000-7000-8000-000000000001',
  due: '2026-03-20',
  created: '2026-03-15',
};

const inputFrom = (task: TaskData): TaskInput => ({
  title: task.title,
  description: task.description,
  status: task.status,
  priority: task.priority,
  category: task.category,
  location: task.location,
  assignee: task.assignee,
  due: task.due,
});

describe('TaskApiService.changedFields', () => {
  it('is empty when the form was not touched', () => {
    // Drives the "nothing to save" short-circuit in the edit sheet.
    expect(TaskApiService.changedFields(baseTask, inputFrom(baseTask))).toEqual({});
  });

  it('carries only the fields that actually changed', () => {
    // The whole point: a title-only edit must not write back status, so it
    // cannot revert a change another admin made since the sheet was opened.
    const patch = TaskApiService.changedFields(baseTask, {
      ...inputFrom(baseTask),
      title: 'Replace failed harddisk',
    });

    expect(patch).toEqual({ title: 'Replace failed harddisk' });
  });

  it('keeps a cleared field in the patch, so the clear is actually sent', () => {
    const patch = TaskApiService.changedFields(baseTask, {
      ...inputFrom(baseTask),
      assignee: null,
      location: '',
      description: '',
    });

    expect(patch).toEqual({ assignee: null, location: '', description: '' });
  });

  it('treats a clear and an overwrite in the same edit independently', () => {
    const patch = TaskApiService.changedFields(baseTask, {
      ...inputFrom(baseTask),
      assignee: null,
      due: '2026-04-01',
    });

    expect(patch).toEqual({ assignee: null, due: '2026-04-01' });
    expect(patch).not.toHaveProperty('location');
  });
});

describe('TaskApiService.mapTask', () => {
  it('maps enums, ids and dates onto the board view-model', () => {
    const task = TaskApiService.mapTask(
      create(TaskSchema, {
        id: baseTask.id,
        title: 'Inspect PDU',
        description: 'Quarterly inspection',
        status: ProtoStatus.READY,
        priority: ProtoPriority.MEDIUM,
        category: ProtoCategory.POWER,
        location: 'Hall A',
        assigneeId: baseTask.assignee!,
        dueDate: timestampFromDate(new Date('2026-03-25T00:00:00Z')),
        created: timestampFromDate(new Date('2026-03-16T09:00:00Z')),
      }),
    );

    expect(task).toEqual({
      id: baseTask.id,
      title: 'Inspect PDU',
      description: 'Quarterly inspection',
      status: 'Ready',
      priority: 'Medium',
      category: 'Power',
      location: 'Hall A',
      assignee: baseTask.assignee,
      due: '2026-03-25',
      created: '2026-03-16',
    });
  });

  it('reads an absent assignee and due date as empty, not as a blank string id', () => {
    const task = TaskApiService.mapTask(create(TaskSchema, { id: baseTask.id, title: 'Bare' }));

    expect(task.assignee).toBeNull();
    expect(task.due).toBe('');
  });

  it('keeps a due date on its calendar day regardless of the local timezone', () => {
    // The column is timestamptz but a due date is a calendar date. Parsing at
    // local midnight would move the stored instant a day back for every UTC+
    // zone, and every save would move it again.
    const task = TaskApiService.mapTask(
      create(TaskSchema, {
        id: baseTask.id,
        dueDate: timestampFromDate(new Date('2026-03-20T00:00:00Z')),
      }),
    );

    expect(task.due).toBe('2026-03-20');
  });
});

describe('TaskApiService enum mapping', () => {
  it('maps every status the API can send', () => {
    expect(TaskApiService.fromProtoStatus(ProtoStatus.READY)).toBe('Ready');
    expect(TaskApiService.fromProtoStatus(ProtoStatus.IN_PROGRESS)).toBe('In Progress');
    expect(TaskApiService.fromProtoStatus(ProtoStatus.REVIEW)).toBe('Review');
    expect(TaskApiService.fromProtoStatus(ProtoStatus.BLOCKED)).toBe('Blocked');
    expect(TaskApiService.fromProtoStatus(ProtoStatus.DONE)).toBe('Done');
  });

  it('falls back on UNSPECIFIED rather than rendering a blank column', () => {
    expect(TaskApiService.fromProtoStatus(ProtoStatus.UNSPECIFIED)).toBe('Ready');
    expect(TaskApiService.fromProtoPriority(ProtoPriority.UNSPECIFIED)).toBe('Medium');
    expect(TaskApiService.fromProtoCategory(ProtoCategory.UNSPECIFIED)).toBe('Other');
  });

  it('falls back and warns on a value this build has no label for', () => {
    // A schema that moved ahead of the frontend: the board still renders, but
    // the mismatch is logged rather than passed off as a legitimate value.
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});

    expect(TaskApiService.fromProtoStatus(99 as ProtoStatus)).toBe('Ready');
    expect(warn).toHaveBeenCalledOnce();

    warn.mockRestore();
  });
});
