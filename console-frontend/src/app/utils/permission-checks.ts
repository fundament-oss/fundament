import {
  computed,
  DestroyRef,
  effect,
  inject,
  signal,
  untracked,
  type Signal,
} from '@angular/core';
import { create } from '@bufbuild/protobuf';
import { Action } from '../../generated/v1/actions_pb';
import { PermissionCheckSchema, type PermissionCheck } from '../../generated/v1/authz_pb';
import PermissionsService from '../permissions.service';
import OrganizationContextService from '../organization-context.service';

/** One question for `can`: may the caller take `action` on the object `objectId`? */
export const check = (action: Action, objectId: string): PermissionCheck =>
  create(PermissionCheckSchema, { action, objectId });

const keyOf = (action: Action, objectId: string) => `${action}:${objectId}`;

export type PermissionLookup = ((action: Action, objectId: string) => boolean) & {
  /** False from the moment a question goes out until its answer is in. */
  settled: Signal<boolean>;
};

/**
 * Authz data reaches the checker a moment after an object is created, so a
 * false right after creation may still turn true. While any answer is false the
 * same questions are asked again this long after the first answer.
 */
export const REASK_DELAYS_MS = [1000, 4000];

/**
 * Asks `can` for every check `source` returns, again whenever the questions
 * `source` returns change, and returns a lookup that is false until a question
 * is first answered and for anything not asked. While changed questions are
 * pending, a question still asked keeps its last answer, so what is on screen
 * does not flash or lose its value; `settled` is false meanwhile. An answer that
 * arrives after a newer question is dropped, and an unchanged set of questions
 * is not asked again. A re-ask only turns false answers true, so a failed one
 * changes nothing.
 * Call it where `effect` may be called.
 */
export default function watchPermissions(source: () => PermissionCheck[]): PermissionLookup {
  const permissions = inject(PermissionsService);
  const answers = signal(new Map<string, boolean>());
  const settled = signal(true);
  let latest = 0;
  let currentQuestions: string | null = null;
  let timers: ReturnType<typeof setTimeout>[] = [];

  const stopReasking = () => {
    timers.forEach((timer) => clearTimeout(timer));
    timers = [];
  };

  const reask = (checks: PermissionCheck[], asked: number) => {
    permissions.can(checks).then((allowed) => {
      if (asked !== latest) return;
      const next = new Map(answers());
      checks.forEach((c, i) => {
        const key = keyOf(c.action, c.objectId);
        next.set(key, next.get(key) === true || allowed[i]);
      });
      answers.set(next);
      if (checks.every((c) => next.get(keyOf(c.action, c.objectId)) === true)) stopReasking();
    });
  };

  effect(() => {
    const checks = source();
    const questions = checks.map((c) => keyOf(c.action, c.objectId)).join(',');
    if (questions === currentQuestions) return;
    currentQuestions = questions;
    stopReasking();
    latest += 1;
    const asked = latest;
    if (checks.length === 0) {
      untracked(() => {
        answers.set(new Map());
        settled.set(true);
      });
      return;
    }
    untracked(() => {
      const stillAsked = new Set(checks.map((c) => keyOf(c.action, c.objectId)));
      answers.set(new Map([...answers()].filter(([key]) => stillAsked.has(key))));
      settled.set(false);
    });
    permissions.can(checks).then((allowed) => {
      if (asked !== latest) return;
      settled.set(true);
      const first = new Map(checks.map((c, i) => [keyOf(c.action, c.objectId), allowed[i]]));
      answers.set(first);
      if (allowed.every(Boolean)) return;
      timers = REASK_DELAYS_MS.map((delay) => setTimeout(() => reask(checks, asked), delay));
    });
  });

  inject(DestroyRef).onDestroy(() => {
    stopReasking();
    // An answer still in flight must not schedule re-asks.
    latest += 1;
  });

  const lookup = (action: Action, objectId: string) =>
    answers().get(keyOf(action, objectId)) === true;
  return Object.assign(lookup, { settled: settled.asReadonly() });
}

/** Whether the caller may take `action` on the current organization. */
export function organizationPermission(action: Action): Signal<boolean> {
  const organizationContext = inject(OrganizationContextService);
  const mayDo = watchPermissions(() => {
    const organizationId = organizationContext.currentOrganizationId();
    return organizationId ? [check(action, organizationId)] : [];
  });
  return computed(() => {
    const organizationId = organizationContext.currentOrganizationId();
    return !!organizationId && mayDo(action, organizationId);
  });
}
