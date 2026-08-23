import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import TaskApiService, { TaskData, TaskInput } from './task-api.service';
import {
  TaskSchema,
  TaskStatus as ProtoStatus,
  TaskPriority as ProtoPriority,
} from '../../generated/v1/task_pb';

const baseTask: TaskData = {
  id: '019dce10-0000-7000-8000-000000000001',
  title: 'Replace broken harddisk',
  description: 'Failed disk in Bay 3',
  status: 'Doing',
  priority: 'Urgent',
  tags: ['hardware'],
  blockedReason: null,
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
  tags: task.tags,
  location: task.location,
  assignee: task.assignee,
  due: task.due,
  blockedReason: task.blockedReason,
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
        status: ProtoStatus.TODO,
        priority: ProtoPriority.MEDIUM,
        tags: ['power', 'hardware'],
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
      status: 'To do',
      priority: 'Medium',
      tags: ['power', 'hardware'],
      blockedReason: null,
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
    expect(TaskApiService.fromProtoStatus(ProtoStatus.TODO)).toBe('To do');
    expect(TaskApiService.fromProtoStatus(ProtoStatus.DOING)).toBe('Doing');
    expect(TaskApiService.fromProtoStatus(ProtoStatus.DONE)).toBe('Done');
  });

  it('falls back on UNSPECIFIED rather than rendering a blank column', () => {
    expect(TaskApiService.fromProtoStatus(ProtoStatus.UNSPECIFIED)).toBe('To do');
  });

  it('reads an unset priority as None, because nobody has prioritized it yet', () => {
    expect(TaskApiService.fromProtoPriority(ProtoPriority.UNSPECIFIED)).toBe('None');
    expect(TaskApiService.fromProtoPriority(ProtoPriority.URGENT)).toBe('Urgent');
  });

  it('falls back and warns on a value this build has no label for', () => {
    // A schema that moved ahead of the frontend: the board still renders, but
    // the mismatch is logged rather than passed off as a legitimate value.
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});

    expect(TaskApiService.fromProtoStatus(99 as ProtoStatus)).toBe('To do');
    expect(warn).toHaveBeenCalledOnce();

    warn.mockRestore();
  });
});
