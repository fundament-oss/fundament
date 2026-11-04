import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { vi } from 'vitest';
import PermissionsService from '../permissions.service';
import { Action } from '../../generated/v1/actions_pb';
import { type PermissionCheck } from '../../generated/v1/authz_pb';
import watchPermissions, { check } from './permission-checks';

describe('watchPermissions', () => {
  it('is false until answered, asks again when the source changes, and drops a stale answer', async () => {
    const resolvers: ((allowed: boolean[]) => void)[] = [];
    const can = vi.fn(
      (_checks: PermissionCheck[]) =>
        new Promise<boolean[]>((resolve) => {
          resolvers.push(resolve);
        }),
    );
    TestBed.configureTestingModule({
      providers: [{ provide: PermissionsService, useValue: { can } }],
    });
    const id = signal('a');
    const lookup = TestBed.runInInjectionContext(() =>
      watchPermissions(() => [check(Action.CLUSTER_CAN_DELETE, id())]),
    );

    TestBed.tick();
    expect(lookup(Action.CLUSTER_CAN_DELETE, 'a')).toBe(false);
    expect(lookup.settled()).toBe(false);

    id.set('b');
    TestBed.tick();
    expect(can).toHaveBeenCalledTimes(2);

    resolvers[1]([true]);
    await Promise.resolve();
    resolvers[0]([false]);
    await Promise.resolve();

    expect(lookup(Action.CLUSTER_CAN_DELETE, 'b')).toBe(true);
    expect(lookup(Action.CLUSTER_CAN_DELETE, 'a')).toBe(false);
    expect(lookup.settled()).toBe(true);
  });

  it('keeps answers for questions still asked and drops the rest while pending', async () => {
    let answer: (allowed: boolean[]) => void = () => {};
    const can = vi
      .fn<(checks: PermissionCheck[]) => Promise<boolean[]>>()
      .mockResolvedValueOnce([true, true])
      .mockImplementationOnce(
        () =>
          new Promise<boolean[]>((resolve) => {
            answer = resolve;
          }),
      );
    TestBed.configureTestingModule({
      providers: [{ provide: PermissionsService, useValue: { can } }],
    });
    const ids = signal(['a', 'b']);
    const lookup = TestBed.runInInjectionContext(() =>
      watchPermissions(() => ids().map((id) => check(Action.CLUSTER_CAN_DELETE, id))),
    );
    TestBed.tick();
    await Promise.resolve();
    await Promise.resolve();
    expect(lookup(Action.CLUSTER_CAN_DELETE, 'a')).toBe(true);
    expect(lookup(Action.CLUSTER_CAN_DELETE, 'b')).toBe(true);

    ids.set(['a', 'c']);
    TestBed.tick();
    expect(lookup(Action.CLUSTER_CAN_DELETE, 'a')).toBe(true);
    expect(lookup(Action.CLUSTER_CAN_DELETE, 'b')).toBe(false);
    expect(lookup(Action.CLUSTER_CAN_DELETE, 'c')).toBe(false);
    expect(lookup.settled()).toBe(false);

    answer([false, true]);
    await Promise.resolve();
    expect(lookup(Action.CLUSTER_CAN_DELETE, 'a')).toBe(false);
    expect(lookup(Action.CLUSTER_CAN_DELETE, 'c')).toBe(true);
    expect(lookup.settled()).toBe(true);
  });

  describe('re-asking', () => {
    function setup(answers: (call: number, checks: PermissionCheck[]) => boolean[]) {
      const can = vi.fn(async (checks: PermissionCheck[]) =>
        answers(can.mock.calls.length, checks),
      );
      TestBed.configureTestingModule({
        providers: [{ provide: PermissionsService, useValue: { can } }],
      });
      const id = signal('a');
      const lookup = TestBed.runInInjectionContext(() =>
        watchPermissions(() => [check(Action.PROJECT_CAN_EDIT, id())]),
      );
      return { can, id, lookup };
    }

    beforeEach(() => vi.useFakeTimers());
    afterEach(() => vi.useRealTimers());

    it('shows a control whose false turns true on a re-ask', async () => {
      const { can, lookup } = setup((call) => [call >= 2]);
      TestBed.tick();
      await vi.advanceTimersByTimeAsync(0);
      expect(lookup(Action.PROJECT_CAN_EDIT, 'a')).toBe(false);

      await vi.advanceTimersByTimeAsync(1000);
      expect(can).toHaveBeenCalledTimes(2);
      expect(lookup(Action.PROJECT_CAN_EDIT, 'a')).toBe(true);

      await vi.advanceTimersByTimeAsync(10_000);
      expect(can).toHaveBeenCalledTimes(2);
    });

    it('re-asks at 1 s and 4 s while false, then stops', async () => {
      const { can } = setup(() => [false]);
      TestBed.tick();
      await vi.advanceTimersByTimeAsync(0);
      await vi.advanceTimersByTimeAsync(1000);
      expect(can).toHaveBeenCalledTimes(2);
      await vi.advanceTimersByTimeAsync(3000);
      expect(can).toHaveBeenCalledTimes(3);
      await vi.advanceTimersByTimeAsync(60_000);
      expect(can).toHaveBeenCalledTimes(3);
    });

    it('makes no extra call when everything is allowed', async () => {
      const { can } = setup(() => [true]);
      TestBed.tick();
      await vi.advanceTimersByTimeAsync(10_000);
      expect(can).toHaveBeenCalledTimes(1);
      expect(vi.getTimerCount()).toBe(0);
    });

    it('cancels pending re-asks when the source changes, and leaves no timers after destroy', async () => {
      const { can, id } = setup(() => [false]);
      TestBed.tick();
      await vi.advanceTimersByTimeAsync(0);
      id.set('b');
      TestBed.tick();
      await vi.advanceTimersByTimeAsync(0);
      expect(can).toHaveBeenCalledTimes(2);

      // Only the second question's re-asks remain: 1 s and 4 s after its answer.
      await vi.advanceTimersByTimeAsync(1000);
      expect(can).toHaveBeenCalledTimes(3);
      expect(can.mock.calls[2][0][0].objectId).toBe('b');

      TestBed.resetTestingModule();
      expect(vi.getTimerCount()).toBe(0);
    });

    it('schedules no re-asks when destroyed with the first answer in flight', async () => {
      const { can } = setup(() => [false]);
      TestBed.tick();
      expect(can).toHaveBeenCalledTimes(1);

      TestBed.resetTestingModule();
      await vi.advanceTimersByTimeAsync(0);
      expect(vi.getTimerCount()).toBe(0);
    });

    it('keeps a true when a re-ask of a mixed batch fails', async () => {
      const can = vi.fn(async (checks: PermissionCheck[]) =>
        can.mock.calls.length === 1 ? [true, false] : checks.map(() => false),
      );
      TestBed.configureTestingModule({
        providers: [{ provide: PermissionsService, useValue: { can } }],
      });
      const lookup = TestBed.runInInjectionContext(() =>
        watchPermissions(() => [
          check(Action.PROJECT_CAN_EDIT, 'a'),
          check(Action.PROJECT_CAN_DELETE, 'a'),
        ]),
      );
      TestBed.tick();
      await vi.advanceTimersByTimeAsync(5000);
      expect(can).toHaveBeenCalledTimes(3);
      expect(lookup(Action.PROJECT_CAN_EDIT, 'a')).toBe(true);
      expect(lookup(Action.PROJECT_CAN_DELETE, 'a')).toBe(false);
    });

    it('does not ask again, and keeps the pending answer, when the questions are unchanged', async () => {
      let answer: (allowed: boolean[]) => void = () => {};
      const can = vi.fn(
        () =>
          new Promise<boolean[]>((resolve) => {
            answer = resolve;
          }),
      );
      TestBed.configureTestingModule({
        providers: [{ provide: PermissionsService, useValue: { can } }],
      });
      const poll = signal(0);
      const lookup = TestBed.runInInjectionContext(() =>
        watchPermissions(() => {
          poll();
          return [check(Action.PROJECT_CAN_EDIT, 'a')];
        }),
      );
      TestBed.tick();
      poll.set(1);
      TestBed.tick();
      poll.set(2);
      TestBed.tick();
      expect(can).toHaveBeenCalledTimes(1);

      answer([true]);
      await vi.advanceTimersByTimeAsync(0);
      expect(lookup(Action.PROJECT_CAN_EDIT, 'a')).toBe(true);
    });
  });
});
