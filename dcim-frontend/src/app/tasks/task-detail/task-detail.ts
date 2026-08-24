import {
  ChangeDetectionStrategy,
  Component,
  computed,
  CUSTOM_ELEMENTS_SCHEMA,
  effect,
  ElementRef,
  inject,
  input,
  output,
  signal,
  viewChild,
} from '@angular/core';
import { NgTemplateOutlet } from '@angular/common';
import { firstValueFrom } from 'rxjs';
import { timestampDate } from '@bufbuild/protobuf/wkt';
import TaskStore, { Note, Task, Technician } from '../../task-management/task-store';
import TaskStatusUi from '../../task-management/task-status-ui.service';
import TaskStatusDialogsComponent from '../../task-management/task-status-dialogs/task-status-dialogs';
import TaskApiService, {
  TaskData,
  TaskPatch,
  TaskPriorityLabel,
  TaskStatusLabel,
} from '../../task-management/task-api.service';
import NoteApiService from '../../inventory/note-api.service';
import type { Note as ProtoNote } from '../../../generated/v1/note_pb';
import ToastService from '../../shared/toast.service';
import connectErrorMessage from '../../../connect/error';
import { taskTags } from '../task-tags';
import { dayLabel, Round } from '../../rounds/round';
import RoundsService from '../../rounds/rounds.service';

/**
 * What a task is called before you have called it anything.
 *
 * Not a nicety: CreateTaskRequest validates the title at min_len 1, so a task
 * whose first written field was a date, a note or a waiting reason has no name
 * the API will take. This is the name it gets until you give it one.
 */
const UNTITLED = 'Untitled';

interface NlddSheet {
  show(): void;
  hide(): void;
}

/**
 * One task, read and changed in the same place.
 *
 * Everything here is the control itself rather than a reading of one, so there
 * is nothing to submit: the field you touch is the whole change, true the moment
 * you set it. That is why this is a component and not a page — the same view
 * opens from the list, and from the button in the bar over whatever else you
 * were looking at.
 */
@Component({
  selector: 'app-task-detail',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  imports: [NgTemplateOutlet, TaskStatusDialogsComponent],
  templateUrl: './task-detail.html',
})
export default class TaskDetailComponent {
  protected readonly store = inject(TaskStore);

  protected readonly ui = inject(TaskStatusUi);

  private readonly taskApi = inject(TaskApiService);

  private readonly noteApi = inject(NoteApiService);

  private readonly roundsService = inject(RoundsService);

  private readonly toast = inject(ToastService);

  /** The task on screen. Null is a task that does not exist yet — see the draft. */
  readonly taskId = input<string | null>(null);

  /** The id it got once it became real, which the host did not hand us. */
  private readonly ownId = signal<string | null>(null);

  /** Whichever of the two names the task on screen. */
  private readonly effectiveId = computed(() => this.taskId() ?? this.ownId());

  /** The sheet around this is the host's; it is told when to close. */
  readonly closed = output<void>();

  /** Asked for the round this task is walked in, which the host opens. */
  readonly openRound = output<void>();

  constructor() {
    // The notes are fetched per task, and refetched on every open: somebody else
    // may have written one since this task was last read.
    effect(() => {
      const id = this.effectiveId();
      if (id !== null) this.loadNotes(id);
    });
  }

  private static relativeTime(date: Date): string {
    const diffMs = Date.now() - date.getTime();
    const days = Math.floor(diffMs / 86_400_000);
    if (days <= 0) return 'Today';
    if (days === 1) return '1 day ago';
    return `${days} days ago`;
  }

  // The same, for the detail sheet's note list.
  readonly notesError = signal<string | null>(null);

  readonly priorities: TaskPriorityLabel[] = ['Urgent', 'High', 'Medium', 'Low', 'None'];

  newNoteText = signal('');

  /**
   * A task that does not exist yet.
   *
   * Held here rather than written the moment you press New, because a task you
   * opened and thought better of should leave nothing behind — not in the list,
   * and not for whoever else is looking at it. The first field you set is what
   * makes it real; see write().
   */
  private readonly draft = signal<Task | null>(null);

  /** Whether this is still a task nobody but you knows about. */
  readonly isDraft = computed(() => this.effectiveId() === null);

  readonly detailTask = computed(() => {
    const id = this.effectiveId();
    if (id === null) return this.draft();
    // Falls back to the draft rather than to nothing: between a task being
    // written and the list holding it there is a moment where neither answers,
    // and an empty sheet for one frame reads as a blink.
    return this.store.tasks().find((t) => t.id === id) ?? this.draft();
  });

  /** Handed to the waiting dialog, so it writes through the same path as a field. */
  readonly writeField = (task: Task, patch: TaskPatch, what: string): void =>
    this.write(task, patch, what);

  /**
   * The status. On a draft it is one more field to set; on a real task it can
   * land on a question, which TaskStatusUi owns.
   */
  setStatus(task: Task, status: TaskStatusLabel, from?: Event): void {
    if (this.isDraft()) {
      this.write(task, { status }, 'the status');
      return;
    }
    this.ui.setStatus(task, status, from);
  }

  /** Opens on a blank task. Called by the host that shows a new one. */
  startDraft(): void {
    this.ownId.set(null);
    this.detailTab.set('info');
    this.detailTitleDraft.set(null);
    this.draft.set({
      id: '',
      title: '',
      description: '',
      status: 'To do',
      priority: 'None',
      tags: [],
      location: '',
      assignee: null,
      due: '',
      created: '',
      blockedReason: null,
      notes: [],
    });
  }

  /**
   * One field of one task, whether or not the task exists yet.
   *
   * On a task that exists this is the patch and nothing more. On one that does
   * not, this field is the moment it becomes real: it is written with what has
   * been set so far, and everything after it is an ordinary patch. Which field
   * does not matter — a task you only gave a date to is still a task, and
   * waiting for a title would lose it.
   */
  private write(task: Task, patch: TaskPatch, what: string): void {
    if (task.id) {
      this.store.patchTask(task, patch, what);
      return;
    }
    const next = { ...task, ...patch };
    this.draft.set(next);
    this.store
      .createTask({ ...next, title: next.title || UNTITLED })
      .then((id) => this.ownId.set(id))
      .catch((err) => {
        // eslint-disable-next-line no-console
        console.error(connectErrorMessage(err));
        this.toast.error(`Could not save ${what} — the task has not been created`);
      });
  }

  readonly deleteDialogEl = viewChild<ElementRef<NlddSheet>>('deleteDialogEl');

  /** The due date, once it is a date. Empty clears it, which the API reads as
   *  "remove". */
  commitDue(task: Task, event: Event): void {
    const due = (event as CustomEvent<{ value?: string }>).detail?.value ?? '';
    if (due === task.due) return;
    this.write(task, { due }, 'the due date');
  }

  setAssignee(task: Task, assignee: string | null): void {
    if (assignee === task.assignee) return;
    this.write(task, { assignee }, 'who it is for');
  }

  setPriority(task: Task, priority: TaskPriorityLabel): void {
    if (priority === task.priority) return;
    this.write(task, { priority }, 'the priority');
  }

  /**
   * The tags, and with them the place. A location used to live in a field of
   * its own, which produced three spellings of one rack; writing here is the
   * migration, so whatever the location said stands among the tags and the old
   * field is cleared.
   */
  commitTags(task: Task, tags: string[]): void {
    const next = [...tags];
    const current = taskTags(task);
    if (next.length === current.length && next.every((tag, i) => tag === current[i])) return;
    this.write(task, { tags: next, location: '' }, 'the tags');
  }

  /**
   * Which half of the task detail you are on. Info is where you change what the
   * task is; Notes is where you read what has been said about it.
   */
  readonly detailTab = signal<'info' | 'notes'>('info');

  /**
   * The description, once you stop typing. An empty one is allowed and means
   * what it says: the API reads an empty description as "remove".
   */
  commitDescription(task: Task, event: Event): void {
    const field = event.target as HTMLElement & { value?: string };
    const description = (field.value ?? '').trim();
    if (description === task.description) return;
    this.write(task, { description }, 'the description');
  }

  /**
   * What the title says while you are typing it, before it is written away.
   * The bar above the sheet reads from this, so it keeps up with the keyboard
   * rather than with the network.
   */
  readonly detailTitleDraft = signal<string | null>(null);

  /**
   * The title, once you stop typing.
   *
   * An empty one is not refused, it is ignored: without a submit there is no
   * moment to refuse anything, and a task with no name cannot be found again.
   * The field is put back to the name the task still has.
   */
  commitTitle(task: Task, event: Event): void {
    this.detailTitleDraft.set(null);
    const field = event.target as HTMLElement & { value?: string };
    const title = (field.value ?? '').trim();
    if (!title) {
      field.value = task.title;
      return;
    }
    if (title === task.title) return;
    this.write(task, { title }, 'the title');
  }

  private loadNotes(id: string): void {
    this.notesError.set(null);
    firstValueFrom(this.noteApi.listNotesForTask(id))
      .then((res) => {
        const notes = res.notes.map((n) => TaskDetailComponent.mapNote(n));
        this.store.tasks.update((tasks) => tasks.map((t) => (t.id === id ? { ...t, notes } : t)));
      })
      .catch((err) => {
        const message = connectErrorMessage(err);
        // eslint-disable-next-line no-console
        console.error(message);
        // Said out loud in the sheet: the note list falls back to whatever was
        // cached from a previous open (or to nothing at all), which is
        // indistinguishable from a task that genuinely has no notes.
        this.notesError.set(message);
      });
  }

  retryLoadNotes(): void {
    const id = this.effectiveId();
    if (id !== null) this.loadNotes(id);
  }

  private static mapNote(n: ProtoNote): Note {
    return {
      author: n.createdBy,
      authorId: n.createdById ? n.createdById : null,
      text: n.body,
      time: TaskDetailComponent.relativeTime(n.created ? timestampDate(n.created) : new Date()),
    };
  }



  /** The round this task is walked in, if it is in one at all. */
  roundOf(task: Task): Round | null {
    return this.roundsService.roundOf().get(task.id) ?? null;
  }

  /**
   * Where the task stands in somebody's day, as a sentence. Most of the time
   * this is the whole answer and the round itself does not have to be opened.
   */
  roundSentence(round: Round, task: TaskData): string {
    const position = round.tasks.findIndex((t) => t.id === task.id) + 1;
    const day = dayLabel(round.day, this.roundsService.todayISO()).toLowerCase();
    return `Task ${position} of ${round.tasks.length} in the round ${round.personName} walks in ${round.datacenter}, ${day}.`;
  }

  /** Why a task is in nobody's round. Which of the three questions is open. */
  readonly roundGap = (task: Task): string => {
    if (!task.assignee) return 'Nobody is assigned to it yet.';
    if (!task.due) return 'It has no due date.';
    return 'None of its tags names a data center.';
  };

  openTechnicianView(): void {
    this.openRound.emit();
  }

  openDeleteDialog(): void {
    if (this.isDraft()) {
      // Nothing written, nothing to confirm: a draft with anything in it is not
      // a draft any more, so there is nothing here anyone could lose.
      this.closed.emit();
      return;
    }
    this.deleteDialogEl()?.nativeElement.show();
  }

  closeDeleteDialog(): void {
    this.deleteDialogEl()?.nativeElement.hide();
  }

  confirmDeleteTask(): void {
    // The id it has, not the one it was opened with: a task written from a draft
    // never got one from its host, and this ran on null and did nothing.
    const id = this.effectiveId();
    this.closeDeleteDialog();
    if (id === null) return;
    firstValueFrom(this.taskApi.deleteTask(id))
      .then(() => {
        this.closed.emit();
        this.store.loadTasks();
      })
      .catch((err) => {
        // eslint-disable-next-line no-console
        console.error(connectErrorMessage(err));
        this.toast.error('Could not delete task');
      });
  }

  /**
   * A note is something you set, so on a task that does not exist yet it makes
   * it real the same way a date or a title does — untitled, and named later.
   */
  addNote(): void {
    const text = this.newNoteText().trim();
    if (!text) return;
    const task = this.detailTask();
    if (!task) return;
    if (task.id) {
      this.postNote(task.id, text);
      return;
    }
    this.store
      .createTask({ ...task, title: task.title || UNTITLED })
      .then((id) => {
        this.ownId.set(id);
        this.postNote(id, text);
      })
      .catch((err) => {
        // eslint-disable-next-line no-console
        console.error(connectErrorMessage(err));
        this.toast.error('Could not add note — the task has not been created');
      });
  }

  private postNote(id: string, text: string): void {
    firstValueFrom(this.noteApi.createNoteForTask(id, text))
      .then(() => {
        this.newNoteText.set('');
        // No toast: the note appearing in the list is the confirmation.
        this.loadNotes(id);
      })
      .catch((err) => {
        // eslint-disable-next-line no-console
        console.error(connectErrorMessage(err));
        this.toast.error('Could not add note');
      });
  }

  // Resolves a note's author onto the roster entry that supplies the avatar.
  // Joined by id, not by display name: two people called "Jan de Vries" would
  // otherwise share one avatar colour, and the first match would win.
  //
  // An author the backend could not attribute is "Unknown", not "Admin" — the
  // note was written by someone outside the directory, which is not the same
  // claim as it having come from an administrator.
  noteAuthor(note: Note): { name: string; tech: Technician | null } {
    const tech = note.authorId ? this.store.getTech(note.authorId) : null;
    return { name: note.author || 'Unknown', tech };
  }
}
