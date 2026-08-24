import { computed, inject, Injectable, signal } from '@angular/core';
import TaskStore, { Task } from './task-store';
import { TaskPatch, TaskStatusLabel } from './task-api.service';

/**
 * Setting a task's status, and the two questions that sometimes go with it.
 *
 * The state lives in a service rather than in a component because the same menu
 * hangs off two things: a row in the list, and the sheet you read a task in.
 * Both can land on a question, and a question that belongs to whichever of them
 * happened to be on screen would be two dialogs that must be kept the same.
 *
 * The dialogs themselves are one component, app-task-status-dialogs, rendered
 * wherever a status menu can be opened.
 */
@Injectable({ providedIn: 'root' })
export default class TaskStatusUi {
  private readonly store = inject(TaskStore);

  /**
   * Opens something modal that was chosen from a menu.
   *
   * Safari does not reliably finish closing the menu before the dialog takes the
   * top layer, and the row the menu hung off then stays lit: the menu never gets
   * round to putting its anchor back. So the menu is closed here by hand, and
   * the dialog goes a microtask later so the two do not land in the same tick.
   * Chrome needs none of this and is unharmed by it.
   */
  private static openFromMenu(from: Event | undefined, show: () => void): void {
    const menu = (from?.target as Element | null)?.closest?.('nldd-menu');
    if (menu instanceof HTMLElement && menu.matches(':popover-open')) menu.hidePopover();
    queueMicrotask(show);
  }

  /** Set by the dialogs component, so this service can open and close them. */
  showWaiting: (() => void) | null = null;

  hideWaiting: (() => void) | null = null;

  showTakeOver: (() => void) | null = null;

  hideTakeOver: (() => void) | null = null;

  // — Status ————————————————————————————————————————————————————————————————

  /**
   * The status, and with it the end of any waiting. You cannot be waiting on
   * something and to-do at the same time, so choosing one of the three lets go
   * of the other; that is why there is no separate way to stop waiting.
   */
  setStatus(task: Task, status: TaskStatusLabel, from?: Event): void {
    if (status !== 'Done' && this.store.somebodyElses(task) && this.store.currentUser()) {
      this.askToTakeOver(task, status, from);
      return;
    }
    if (task.status === status && task.blockedReason === null) return;
    const patch: TaskPatch = { status };
    if (task.blockedReason !== null) patch.blockedReason = null;
    this.store.patchTask(task, patch, 'the status');
  }

  // — Taking a task over ————————————————————————————————————————————————————

  /** The task and the status waiting on an answer to "then it becomes yours". */
  readonly takeOver = signal<{ task: Task; status: TaskStatusLabel } | null>(null);

  /** What the handover costs, spelled out: who has it now and what happens. */
  readonly takeOverExplanation = computed(() => {
    const pending = this.takeOver();
    if (!pending) return '';
    return `It is assigned to ${this.store.assigneeName(pending.task)}. Setting it to ${pending.status} assigns it to you.`;
  });

  /**
   * Setting the status of a task that is somebody else's.
   *
   * To do and Doing are states the person holding it is in, so choosing one for
   * a task that is not yours only makes sense if you are taking it over. That is
   * a second change, to the assignee, and a status menu is no place to make one
   * quietly — hence the question. Done is not the same: closing somebody else's
   * task does not make it yours.
   */
  private askToTakeOver(task: Task, status: TaskStatusLabel, from?: Event): void {
    this.takeOver.set({ task, status });
    TaskStatusUi.openFromMenu(from, () => this.showTakeOver?.());
  }

  cancelTakeOver(): void {
    this.hideTakeOver?.();
    this.takeOver.set(null);
  }

  confirmTakeOver(): void {
    const pending = this.takeOver();
    this.cancelTakeOver();
    if (!pending) return;
    const me = this.store.currentUser()?.id ?? null;
    this.store.patchTask(
      pending.task,
      { status: pending.status, assignee: me, blockedReason: null },
      'who it is for',
    );
  }

  // — Waiting ———————————————————————————————————————————————————————————————

  /**
   * Which task the waiting dialog is about.
   *
   * Looked up again by id rather than read off the captured object, because the
   * list reloads underneath and a held task would answer with what was true when
   * the dialog opened. It falls back to what it was opened with, for the one
   * task the list cannot hold: one that has not been written yet.
   */
  private readonly waitingSubject = signal<Task | null>(null);

  readonly waitingTask = computed(() => {
    const held = this.waitingSubject();
    if (!held) return null;
    return this.store.tasks().find((task) => task.id === held.id) ?? held;
  });

  /**
   * Who writes what this dialog decides.
   *
   * A task that exists is patched. One that does not has to be written first,
   * and only its own sheet knows how — so that sheet hands its writer in when it
   * opens the dialog. Waiting on something is a fact like any other, so it makes
   * a task real the same way a date or a title does.
   */
  private waitingWriter: ((task: Task, patch: TaskPatch, what: string) => void) | null = null;

  /** Taken before the dialog closes, because closing lets go of the writer. */
  private takeWaitingWriter(): (task: Task, patch: TaskPatch, what: string) => void {
    const write = this.waitingWriter;
    if (write) return write;
    return (task, patch, what) => this.store.patchTask(task, patch, what);
  }

  /** What the task is waiting on, while the dialog is open. */
  readonly waitingDraft = signal('');

  /** Which of the three kinds of waiting the dialog is on. */
  readonly waitingChoice = signal<'start' | 'finish' | 'other'>('other');

  /**
   * Who the waiting is on, while the dialog is open.
   *
   * On somebody else's task that is the person who has it and the radios say so
   * by name. On your own there is nobody to name yet, so you point one out, and
   * the task becomes theirs: waiting for a person to start is the same fact as
   * the work being on their list.
   */
  readonly waitingWho = signal<string | null>(null);

  /** The person the first two options are about, if the task already names one. */
  waitingPerson(task: Task): string | null {
    return this.store.somebodyElses(task) ? this.store.assigneeName(task) : null;
  }

  /** Everyone the waiting could be on. Waiting for yourself is what To do says. */
  readonly otherTechnicians = computed(() => {
    const me = this.store.currentUser()?.id;
    return this.store.technicians().filter((tech) => tech.id !== me);
  });

  /** Nothing to save while an option about a person has no person. */
  canSaveWaiting(task: Task): boolean {
    if (this.waitingChoice() === 'other') return true;
    return this.waitingPerson(task) !== null || this.waitingWho() !== null;
  }

  openWaitingDialog(
    task: Task,
    from?: Event,
    write?: (task: Task, patch: TaskPatch, what: string) => void,
  ): void {
    this.waitingSubject.set(task);
    this.waitingWriter = write ?? null;
    this.waitingDraft.set(task.blockedReason ?? '');
    this.waitingWho.set(null);
    // Opens on what it already is: a reason of its own, or where the person who
    // has it stands with it.
    if (task.blockedReason !== null || !this.store.somebodyElses(task)) {
      this.waitingChoice.set('other');
    } else {
      this.waitingChoice.set(task.status === 'Doing' ? 'finish' : 'start');
    }
    TaskStatusUi.openFromMenu(from, () => this.showWaiting?.());
  }

  closeWaitingDialog(): void {
    this.hideWaiting?.();
    this.waitingSubject.set(null);
    this.waitingWriter = null;
  }

  /**
   * What the task is waiting on.
   *
   * Two of the three are where the person who has it stands with it, which the
   * app can read back off the status afterwards; only the third is something
   * nobody could have known. Picking one of the first two clears any reason,
   * because a task waits on one thing at a time.
   */
  commitWaiting(task: Task): void {
    const choice = this.waitingChoice();
    if (choice !== 'other' && !this.canSaveWaiting(task)) return;
    const write = this.takeWaitingWriter();
    this.closeWaitingDialog();
    if (choice === 'other') {
      const reason = this.waitingDraft().trim();
      if (reason === (task.blockedReason ?? '')) return;
      write(task, { blockedReason: reason }, 'what it is waiting on');
      return;
    }
    const status: TaskStatusLabel = choice === 'finish' ? 'Doing' : 'To do';
    const patch: TaskPatch = { status, blockedReason: null };
    // Handing it over is part of the same sentence: you cannot wait for somebody
    // to start a task that is not theirs.
    const who = this.waitingWho();
    if (who !== null && who !== task.assignee) patch.assignee = who;
    write(task, patch, 'what it is waiting on');
  }
}
