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
/** The four things a copy of app-task-status-dialogs can be asked to do. */
export interface DialogHandles {
  showWaiting: () => void;
  hideWaiting: () => void;
  showTakeOver: () => void;
  hideTakeOver: () => void;
}

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

  /**
   * Every app-task-status-dialogs on screen, by the element it sits in.
   *
   * There is more than one: the tasks page has a copy, and so does the task
   * beside it, which is in a sheet. A single pair of handles was therefore
   * whichever copy had rendered last, so choosing Waiting on a row opened the
   * window inside the sheet. A closed sheet shows nothing, and a modal in it
   * still takes the top layer, which is a page gone blank that swallows every
   * click.
   */
  private readonly copies = new Map<HTMLElement, DialogHandles>();

  /** The copy a question was opened in, so the same one closes it again. */
  private openIn: DialogHandles | null = null;

  registerDialogs(host: HTMLElement, handles: DialogHandles): void {
    this.copies.set(host, handles);
  }

  unregisterDialogs(host: HTMLElement): void {
    this.copies.delete(host);
  }

  /**
   * Which copy a menu belongs to: the one you can see, and of those the one
   * nearest the menu in the tree. Visibility settles the sheet, which is hidden
   * while it is closed; nearness settles the other direction, because with the
   * sheet open both copies are on screen and the menu in it means the one in it.
   */
  private copyFor(from?: Event): DialogHandles | null {
    const trigger = (from?.target as Element | null) ?? null;
    let best: DialogHandles | null = null;
    let bestDepth = -1;
    let bestNesting = Infinity;
    this.copies.forEach((handles, host) => {
      if (!host.checkVisibility()) return;
      const depth = trigger ? TaskStatusUi.sharedDepth(trigger, host) : 0;
      const nesting = TaskStatusUi.depthOf(host);
      // A menu in the sheet shares more of its ancestry with the copy in there,
      // so that one wins on the first number. A menu on the page shares exactly
      // as much with both, because both copies hang under the page: the second
      // number then picks the one that is not tucked away inside the sheet.
      if (depth > bestDepth || (depth === bestDepth && nesting < bestNesting)) {
        bestDepth = depth;
        bestNesting = nesting;
        best = handles;
      }
    });
    return best;
  }

  /** How far down the tree an element sits. */
  private static depthOf(el: Element): number {
    let depth = 0;
    for (let e: Element | null = el; e; e = e.parentElement) depth += 1;
    return depth;
  }

  /** How many elements deep the two sit under the same ancestor. */
  private static sharedDepth(a: Element, b: Element): number {
    const ancestors = (el: Element): Element[] => {
      const out: Element[] = [];
      for (let e: Element | null = el; e; e = e.parentElement) out.unshift(e);
      return out;
    };
    const one = ancestors(a);
    const other = ancestors(b);
    let i = 0;
    while (i < one.length && i < other.length && one[i] === other[i]) i += 1;
    return i;
  }

  // — Status ————————————————————————————————————————————————————————————————

  /**
   * The status, and with it the end of any waiting. You cannot be waiting on
   * something and to-do at the same time, so choosing one of the three lets go
   * of the other; that is why there is no separate way to stop waiting.
   */
  setStatus(task: Task, status: TaskStatusLabel, from?: Event): void {
    // Before the question below, not after: picking the state a task is already
    // in changes nothing, and asking who that non-change is for is a question
    // with nothing behind it. Taking a task over is not lost by this — the
    // sheet has a field for who it is assigned to, which is where that belongs.
    if (task.status === status && task.blockedReason === null) return;
    if (status !== 'Done' && this.store.somebodyElses(task) && this.store.currentUser()) {
      this.askToTakeOver(task, status, from);
      return;
    }
    const patch: TaskPatch = { status };
    if (task.blockedReason !== null) patch.blockedReason = null;
    this.store.patchTask(task, patch, 'the status');
  }

  // — Taking a task over ————————————————————————————————————————————————————

  /** The task and the status waiting on an answer to "then it becomes yours". */
  readonly takeOver = signal<{ task: Task; status: TaskStatusLabel } | null>(null);

  /** Whose task it is, for the question and for the button that answers it. */
  readonly takeOverAssignee = computed(() => {
    const pending = this.takeOver();
    return pending ? this.store.assigneeName(pending.task) : '';
  });

  /** The question: the change you asked for, and who it would be for. */
  readonly takeOverQuestion = computed(() => {
    const pending = this.takeOver();
    if (!pending) return '';
    return `Set it to ${pending.status} for ${this.takeOverAssignee()}?`;
  });

  /** The other answer, so the button below is not the only place it is said. */
  readonly takeOverExplanation = computed(() =>
    this.takeOver() ? 'The task is theirs. Taking it over assigns it to you instead.' : '',
  );

  /** What the button that keeps it with them says. */
  readonly takeOverKeepAction = computed(() => `Set it for ${this.takeOverAssignee()}`);

  /**
   * Setting the status of a task that is somebody else's.
   *
   * To do and Doing are states the person holding it is in, so choosing one on
   * a task that is not yours reads two ways: you know they have started, or you
   * are taking it off them. The second is a change of assignee as well, and a
   * status menu is no place to make one on the quiet — hence the question.
   *
   * Recording it for them is the answer that leaves everything else alone, so
   * that is the one the dialog opens on. Done is not asked at all: closing
   * somebody else's task does not make it yours.
   */
  private askToTakeOver(task: Task, status: TaskStatusLabel, from?: Event): void {
    this.takeOver.set({ task, status });
    this.openIn = this.copyFor(from);
    TaskStatusUi.openFromMenu(from, () => this.openIn?.showTakeOver());
  }

  cancelTakeOver(): void {
    this.openIn?.hideTakeOver();
    this.takeOver.set(null);
  }

  /** Their task, their status: only the status moves, the assignee stays. */
  confirmForAssignee(): void {
    const pending = this.takeOver();
    this.cancelTakeOver();
    if (!pending) return;
    this.store.patchTask(
      pending.task,
      { status: pending.status, blockedReason: null },
      'the status',
    );
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
    this.openIn = this.copyFor(from);
    TaskStatusUi.openFromMenu(from, () => this.openIn?.showWaiting());
  }

  closeWaitingDialog(): void {
    this.openIn?.hideWaiting();
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
