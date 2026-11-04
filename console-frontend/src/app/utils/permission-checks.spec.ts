import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { vi } from 'vitest';
import PermissionsService from '../permissions.service';
import { Action, type PermissionCheck } from '../../generated/v1/authz_pb';
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
      watchPermissions(() => [check(Action.DELETE_CLUSTER, id())]),
    );

    TestBed.tick();
    expect(lookup(Action.DELETE_CLUSTER, 'a')).toBe(false);
    expect(lookup.settled()).toBe(false);

    id.set('b');
    TestBed.tick();
    expect(can).toHaveBeenCalledTimes(2);

    resolvers[1]([true]);
    await Promise.resolve();
    resolvers[0]([false]);
    await Promise.resolve();

    expect(lookup(Action.DELETE_CLUSTER, 'b')).toBe(true);
    expect(lookup(Action.DELETE_CLUSTER, 'a')).toBe(false);
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
        watchPermissions(() => [check(Action.EDIT_PROJECT, id())]),
      );
      return { can, id, lookup };
    }

    beforeEach(() => vi.useFakeTimers());
    afterEach(() => vi.useRealTimers());

    it('shows a control whose false turns true on a re-ask', async () => {
      const { can, lookup } = setup((call) => [call >= 2]);
      TestBed.tick();
      await vi.advanceTimersByTimeAsync(0);
      expect(lookup(Action.EDIT_PROJECT, 'a')).toBe(false);

      await vi.advanceTimersByTimeAsync(1000);
      expect(can).toHaveBeenCalledTimes(2);
      expect(lookup(Action.EDIT_PROJECT, 'a')).toBe(true);

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

    it('keeps prior answers when a re-ask fails', async () => {
      const { can, lookup } = setup((call, checks) =>
        call === 1 ? checks.map(() => true) : checks.map(() => false),
      );
      TestBed.tick();
      await vi.advanceTimersByTimeAsync(0);
      expect(lookup(Action.EDIT_PROJECT, 'a')).toBe(true);
      expect(can).toHaveBeenCalledTimes(1);
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
          check(Action.EDIT_PROJECT, 'a'),
          check(Action.DELETE_PROJECT, 'a'),
        ]),
      );
      TestBed.tick();
      await vi.advanceTimersByTimeAsync(5000);
      expect(can).toHaveBeenCalledTimes(3);
      expect(lookup(Action.EDIT_PROJECT, 'a')).toBe(true);
      expect(lookup(Action.DELETE_PROJECT, 'a')).toBe(false);
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
          return [check(Action.EDIT_PROJECT, 'a')];
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
      expect(lookup(Action.EDIT_PROJECT, 'a')).toBe(true);
    });
  });
});
