import { createClient } from '@connectrpc/connect';
import { TaskService } from '../../generated/v1/task_pb';
import createDemoTransport from './mock-transport';
import TaskApiService from '../task-management/task-api.service';

/**
 * The demo transport stands in for the API, so a clear has to clear there too:
 * an admin walking through the demo must see the same result the server gives.
 */
describe('demo transport updateTask', () => {
  const client = () => createClient(TaskService, createDemoTransport());

  const firstTaskWith = async (
    c: ReturnType<typeof client>,
    pick: (t: ReturnType<typeof TaskApiService.mapTask>) => boolean,
  ) => {
    const res = await c.listTasks({});
    const found = res.tasks.map((t) => TaskApiService.mapTask(t)).find(pick);
    if (!found) throw new Error('no matching task in the demo fixtures');
    return found;
  };

  const reload = async (c: ReturnType<typeof client>, id: string) =>
    TaskApiService.mapTask((await c.getTask({ id })).task!);

  it('clears the last tag when clear_tags says so', async () => {
    const c = client();
    const task = await firstTaskWith(c, (t) => t.tags.length > 0);

    await c.updateTask({ id: task.id, tags: [], clearTags: true });

    expect((await reload(c, task.id)).tags).toEqual([]);
  });

  it('leaves the tags alone when an empty list arrives without the flag', async () => {
    const c = client();
    const task = await firstTaskWith(c, (t) => t.tags.length > 0);

    await c.updateTask({ id: task.id, tags: [] });

    expect((await reload(c, task.id)).tags).toEqual(task.tags);
  });

  it('unsets the blocked reason on clear, rather than storing an empty one', async () => {
    const c = client();
    const task = await firstTaskWith(c, (t) => t.blockedReason !== null);

    await c.updateTask({ id: task.id, clearBlockedReason: true });

    // null, not '': '' would leave the task waiting with nothing typed.
    expect((await reload(c, task.id)).blockedReason).toBeNull();
  });

  it('keeps a task waiting when the reason is emptied but not cleared', async () => {
    const c = client();
    // Blocked here rather than picked out of the fixtures: the demo store is
    // shared, so a sibling test may already have cleared the one that ships
    // blocked.
    const task = await firstTaskWith(c, () => true);
    await c.updateTask({ id: task.id, blockedReason: 'part on order' });

    await c.updateTask({ id: task.id, blockedReason: '' });

    // '' is a real value: waiting, with nothing typed. Only clear_blocked_reason
    // takes the task off hold.
    expect((await reload(c, task.id)).blockedReason).toBe('');
  });

  it('does not make an unblocked task waiting when some other field is patched', async () => {
    const c = client();
    const task = await firstTaskWith(c, (t) => t.blockedReason === null);

    await c.updateTask({ id: task.id, title: 'Renamed' });

    const after = await reload(c, task.id);
    expect(after.title).toBe('Renamed');
    expect(after.blockedReason).toBeNull();
  });
});
