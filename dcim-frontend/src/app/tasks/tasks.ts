import {
  Component,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
  OnInit,
  OnDestroy,
  signal,
  computed,
  effect,
  untracked,
  inject,
  viewChild,
  ElementRef,
  TemplateRef,
  AfterViewInit,
} from '@angular/core';
import { NgTemplateOutlet } from '@angular/common';
import { toSignal } from '@angular/core/rxjs-interop';
import { ActivatedRoute, convertToParamMap, Router } from '@angular/router';
import { firstValueFrom, Observable } from 'rxjs';
import { timestampDate } from '@bufbuild/protobuf/wkt';
import {
  CdkDropList,
  CdkDropListGroup,
  CdkDrag,
  CdkDragPlaceholder,
  CdkDragDrop,
} from '@angular/cdk/drag-drop';
import type { Note as ProtoNote } from '../../generated/v1/note_pb';
import DropdownSyncDirective from '../shared/dropdown-sync.directive';
import TaskApiService, {
  TaskData,
  TaskInput,
  TaskPatch,
  TaskPriorityLabel,
  TaskStatusLabel,
} from '../task-management/task-api.service';
import UserApiService, { RosterUser } from '../task-management/user-api.service';
import NoteApiService from '../inventory/note-api.service';
import settledPool from '../shared/settled-pool';
import ToastService from '../shared/toast.service';
import connectErrorMessage from '../../connect/error';
import SecondaryNavService from '../shell/secondary-nav.service';
import { TASKS_PATH, viewSlug } from './task-views';

type Technician = RosterUser;

interface Note {
  author: string;
  // The author's roster id, or null for an unattributed note (written by someone
  // outside the directory). Kept alongside the display name so the avatar can be
  // resolved by id rather than by matching names.
  authorId: string | null;
  text: string;
  time: string;
}

interface Task extends TaskData {
  notes: Note[];
}

interface StatusStyle {
  bg: string;
  text: string;
  dot: string;
  kanbanAccent: string;
  kanbanBorder: string;
  /** `color` for an `nldd-tag`. */
  tagColor: string;
}

interface PriorityStyle {
  bg: string;
  text: string;
  dot: string;
  ring: string;
  /** `color` for an `nldd-tag`. */
  tagColor: string;
}

interface NlddSheet extends HTMLElement {
  show(): void;
  hide(): void;
}

/**
 * What a row in the menu points at. The first four are views: they cut across
 * the board and answer a question ("what is not decided yet", "what has to
 * happen today", "what is not mine to move"). The last three are the columns
 * themselves.
 */
type MenuKind = 'all' | 'inbox' | 'today' | 'waiting' | 'status' | 'priority' | 'tag';

// Half the width of the filters <aside> (w-60 = 240px), so the toast centers
// over the main content area next to it instead of the full viewport.
const TOAST_SIDEBAR_OFFSET_PX = 120;

// Ceiling on in-flight requests for the bulk actions. Select-all over a large
// board would otherwise put one request per task on the wire at once.
const BULK_CONCURRENCY = 6;

@Component({
  selector: 'app-tasks',
  templateUrl: './tasks.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [
    NgTemplateOutlet,
    DropdownSyncDirective,
    CdkDropListGroup,
    CdkDropList,
    CdkDrag,
    CdkDragPlaceholder,
  ],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: {
    // No styling of its own. The page inside paints the surface and owns the
    // layout, and styles.css takes this element out of the flow entirely
    // (display: contents), so it cannot come between the pane and the page.
    '(document:keydown.escape)': 'onEscape()',
  },
})
export default class TasksComponent implements OnInit, OnDestroy, AfterViewInit, OnDestroy {
  private readonly secondaryNav = inject(SecondaryNavService);

  /** This section's menu, handed to the shell for as long as the page is open. */
  private readonly secondaryNavTemplate = viewChild.required<TemplateRef<unknown>>('secondaryNav');

  ngAfterViewInit(): void {
    this.secondaryNav.set(this.secondaryNavTemplate());
  }

  private readonly taskApi = inject(TaskApiService);

  private readonly userApi = inject(UserApiService);

  private readonly noteApi = inject(NoteApiService);

  protected readonly toast = inject(ToastService);

  readonly technicians = signal<Technician[]>([]);

  // False until ListUsers has actually answered. An empty roster because the
  // response has not landed (or failed) is not the same fact as an assignee
  // having left the directory, and assigneeMissing() must not report the first
  // as the second — see assigneeUnresolved().
  readonly rosterLoaded = signal(false);

  tasks = signal<Task[]>([]);

  // Set when the board could not be loaded, so an API failure reads as an error
  // rather than as a legitimately empty board.
  readonly loadError = signal<string | null>(null);

  // The same, for the detail sheet's note list.
  readonly notesError = signal<string | null>(null);

  // The signed-in user's roster entry, for the note composer's avatar. Null when
  // the caller has no directory entry — GetCurrentUser answers NotFound then,
  // which is an ordinary provisioning state and not worth a toast here: the
  // board still works, and any note they write comes out unattributed.
  readonly currentUser = signal<Technician | null>(null);

  constructor() {
    // A selection survives no further than the view it was made in. Half of it
    // would be off screen after a move to another view, and the bulk actions
    // act on the whole set: deleting six tasks of which you can see two is the
    // kind of thing you only notice afterwards.
    effect(() => {
      this.menuSelection();
      this.selectedTasks.set(new Set());
    });

    // Holds on to rows that stop belonging here while you are looking at them,
    // and lets go of all of them the moment you go somewhere else. Leaving is
    // the clean-up: that is why the view is compared as well as the ids, so
    // arriving somewhere new never counts as "these rows just dropped out".
    effect(() => {
      const view = JSON.stringify(this.menuSelection());
      const ids = new Set(this.matchingTasks().map((t) => t.id));
      untracked(() => {
        if (view === this.seenView()) {
          const dropped = [...this.seenIds()].filter((id) => !ids.has(id));
          if (dropped.length) {
            this.heldIds.update((held) => new Set([...held, ...dropped]));
          }
        } else {
          this.seenView.set(view);
          this.heldIds.set(new Set());
        }
        this.seenIds.set(ids);
      });
    });
  }

  ngOnInit(): void {
    this.toast.offsetPx.set(TOAST_SIDEBAR_OFFSET_PX);
    this.loadCurrentUser();
    this.loadUsers();
    this.loadTasks();
  }

  ngOnDestroy(): void {
    this.toast.offsetPx.set(0);
    this.secondaryNav.clear(this.secondaryNavTemplate());
  }

  private loadCurrentUser(): void {
    firstValueFrom(this.userApi.getCurrentUser())
      .then((res) => this.currentUser.set(res.user ? UserApiService.mapUser(res.user) : null))
      .catch((err) => {
        // eslint-disable-next-line no-console
        console.error(connectErrorMessage(err));
        this.currentUser.set(null);
      });
  }

  private loadUsers(): void {
    firstValueFrom(this.userApi.listUsers())
      .then((res) => {
        this.technicians.set(res.users.map((u) => UserApiService.mapUser(u)));
        this.rosterLoaded.set(true);
      })
      .catch((err) => {
        // eslint-disable-next-line no-console
        console.error(connectErrorMessage(err));
        // Left false deliberately: without a roster the board cannot tell a
        // departed assignee from one it merely failed to look up, so it says
        // neither rather than picking the alarming reading.
        this.rosterLoaded.set(false);
        this.toast.show('Could not load the technician roster');
      });
  }

  // Retries everything the board loads on entry, the current user included —
  // otherwise a transient GetCurrentUser failure would leave the note
  // composer's avatar unresolved until a full page reload.
  retryLoad(): void {
    this.loadCurrentUser();
    this.loadUsers();
    this.loadTasks();
  }

  // Called after every mutation (each save, each kanban drop, each bulk action).
  // It rebuilds the whole board, so already-fetched notes are carried over by id
  // rather than reset: a mutation landing while the detail sheet is open would
  // otherwise empty its note list, even though no note changed. ListTasks does
  // not carry notes, so a task the sheet has never been opened for keeps an
  // empty list until openDetail() fetches them.
  private loadTasks(): void {
    firstValueFrom(this.taskApi.listTasks())
      .then((res) => {
        this.tasks.update((previous) => {
          const notesById = new Map(previous.map((t) => [t.id, t.notes]));
          return res.tasks.map((t) => ({
            ...TaskApiService.mapTask(t),
            notes: notesById.get(t.id) ?? [],
          }));
        });
        this.loadError.set(null);
      })
      .catch((err) => {
        const message = connectErrorMessage(err);
        // eslint-disable-next-line no-console
        console.error(message);
        this.loadError.set(message);
        this.toast.show('Could not load tasks');
      });
  }

  readonly statusStyles: Record<string, StatusStyle> = {
    'To do': {
      bg: 'bg-slate-100 dark:bg-gray-800',
      text: 'text-slate-600 dark:text-gray-300',
      dot: 'bg-slate-400',
      kanbanAccent: 'bg-slate-400',
      kanbanBorder: 'border-slate-200 dark:border-gray-800',
      tagColor: 'neutral',
    },
    Doing: {
      bg: 'bg-indigo-50 dark:bg-indigo-950',
      text: 'text-indigo-700 dark:text-indigo-300',
      dot: 'bg-indigo-500',
      kanbanAccent: 'bg-indigo-500',
      kanbanBorder: 'border-indigo-200 dark:border-indigo-800',
      tagColor: 'donkerblauw',
    },
    Done: {
      bg: 'bg-emerald-50 dark:bg-emerald-950',
      text: 'text-emerald-700 dark:text-emerald-300',
      dot: 'bg-emerald-500',
      kanbanAccent: 'bg-emerald-500',
      kanbanBorder: 'border-emerald-200 dark:border-emerald-800',
      tagColor: 'success',
    },
  };

  readonly priorityStyles: Record<string, PriorityStyle> = {
    Urgent: {
      bg: 'bg-red-50 dark:bg-red-950',
      text: 'text-red-700 dark:text-red-300',
      dot: 'bg-red-500',
      ring: 'ring-red-200/80 dark:ring-red-800/80',
      tagColor: 'critical',
    },
    High: {
      bg: 'bg-orange-50 dark:bg-orange-950',
      text: 'text-orange-700 dark:text-orange-300',
      dot: 'bg-orange-500',
      ring: 'ring-orange-200/80 dark:ring-orange-800/80',
      tagColor: 'oranje',
    },
    Medium: {
      bg: 'bg-yellow-50 dark:bg-yellow-950',
      text: 'text-yellow-700 dark:text-yellow-300',
      dot: 'bg-yellow-400',
      ring: 'ring-yellow-200/80 dark:ring-yellow-800/80',
      tagColor: 'geel',
    },
    Low: {
      bg: 'bg-slate-100 dark:bg-gray-800',
      text: 'text-slate-500 dark:text-gray-400',
      dot: 'bg-slate-400',
      ring: 'ring-slate-200/80 dark:ring-gray-700/80',
      tagColor: 'neutral',
    },
  };

  readonly kanbanColumns: TaskStatusLabel[] = ['To do', 'Doing', 'Done'];

  readonly priorities: TaskPriorityLabel[] = ['Urgent', 'High', 'Medium', 'Low', 'None'];

  /** The picker offers all five; the menu leaves None out. */
  readonly menuPriorities: TaskPriorityLabel[] = ['Urgent', 'High', 'Medium', 'Low'];

  /** Today's date as the ISO day the task dates are stored in. */
  private readonly todayISO = () => new Date().toISOString().slice(0, 10);

  /**
   * What has to happen today: everything whose due date has passed or is today,
   * plus everything urgent. The urgent half is a deliberate stretch — urgent
   * says act now and that outranks a date further out — but it is also the half
   * that can silently fill this view up: an urgent task without a date never
   * leaves it. If that starts happening, give urgent a date instead of widening
   * the view.
   */
  isToday(task: TaskData): boolean {
    if (task.status === 'Done') return false;
    return (task.due !== '' && task.due <= this.todayISO()) || task.priority === 'Urgent';
  }

  readonly todayCount = computed(() => this.tasks().filter((t) => this.isToday(t)).length);

  /**
   * What nobody has decided about yet. Without an assignee a task has no owner,
   * without a due date it has no when, and either gap stops it from being work
   * anybody can pick up. Both gaps at once is the classic case: something got
   * reported and dropped here. Done tasks are out, there is nothing left to
   * decide about them.
   */
  readonly isInbox = (task: TaskData): boolean => {
    if (task.status === 'Done') return false;
    return !task.assignee || task.due === '';
  };

  readonly inboxCount = computed(() => this.tasks().filter((t) => this.isInbox(t)).length);

  /**
   * What is not yours to move. Somebody else's to-do is your waiting-for, and a
   * task stuck on a part rather than a person waits just as hard. Same rule as
   * the line under the title, so the view and the row can never disagree.
   */
  isWaiting(task: TaskData): boolean {
    return this.holdReason(task) !== null;
  }

  readonly waitingCount = computed(() => this.tasks().filter((t) => this.isWaiting(t)).length);

  /**
   * Whether a task is yours. An unknown current user is not the same fact as
   * "assigned to somebody else": the answer is unavailable, so nothing is
   * filtered out on it and you see a full list rather than a convincing empty
   * one.
   */
  private isMine(task: TaskData): boolean {
    const me = this.currentUser()?.id;
    if (!me) return true;
    return task.assignee === me;
  }

  /**
   * Whether a task belongs under a status row in the menu. To do and Doing are
   * your own queues: somebody else's to-do is your waiting-for, so it belongs
   * under Waiting and would otherwise be counted twice, once in each. Done is
   * everybody's, because finished work is finished whoever did it.
   */
  inStatusView(task: TaskData, status: TaskStatusLabel): boolean {
    if (task.status !== status) return false;
    if (status === 'Done') return true;
    return this.isMine(task);
  }

  /**
   * Why a task is not yours to move. Being stuck on something that is not a
   * person is stored on the task; lying with a person is not stored at all but
   * read off the assignee, because it depends on who is looking: the task you
   * are waiting for is the task somebody else has to do. Returns nothing when
   * the work is yours, or finished.
   */
  holdReason(task: TaskData): string | null {
    if (task.status === 'Done') return null;

    if (task.blockedReason !== null) {
      return task.blockedReason ? `Blocked: ${task.blockedReason}` : 'Blocked';
    }

    if (!task.assignee) return null;

    const me = this.currentUser()?.id;
    if (me && task.assignee === me) return null;

    const who = this.getTech(task.assignee);
    if (!who) return null;

    // Who has it, not what that means for you. Whether they have started is the
    // icon's job now, so this line does not have to carry it in two phrasings.
    return `Assigned to ${who.name}`;
  }

  /**
   * Whose task it is, in words, under the title. It replaces a status column and
   * a face that each said half a fact: "Doing" did not say by whom, and an avatar
   * did not say what was being done. Now the icon in front says how far the work
   * has got and this line says who has it, so neither repeats the other.
   */
  stateLine(task: TaskData): string {
    const hold = this.holdReason(task);
    if (hold) return hold;

    const me = this.currentUser()?.id;
    const who = this.getTech(task.assignee);

    if (task.status === 'Done') {
      return who && task.assignee !== me ? `Done by ${who.name}` : 'Done';
    }

    // Assigned to somebody the roster cannot name. Two different facts, and
    // neither of them is "unassigned": the task is spoken for either way.
    if (this.assigneeUnresolved(task)) return 'Assigned, directory unavailable';
    if (this.assigneeMissing(task)) return 'Assigned, no longer in the directory';

    if (!task.assignee) return 'Unassigned';

    return task.status;
  }

  /**
   * The design system draws priority as a count: one exclamation mark in a
   * circle, then two, then three. Urgent is the same three, filled, so the
   * column keeps reading as a count rather than switching shape at the top. An
   * empty circle is the zero of that series. No color: the number of marks says
   * the level, and coloring it too would say the same thing twice. Alias names,
   * because they say what the icon is for.
   */
  readonly priorityIcon = (priority: TaskPriorityLabel): string => {
    const map: Record<TaskPriorityLabel, string> = {
      Urgent: 'high-priority-filled',
      High: 'high-priority',
      Medium: 'medium-priority',
      Low: 'low-priority',
      None: 'no-priority',
    };
    return map[priority];
  };

  /**
   * Status as one ring three times over: empty, a second ring inside it, ticked.
   * The three read as one series that way, which a set of unrelated pictures
   * would not. Alias names, because those say what the row is rather than what
   * the icon draws.
   *
   * Every name has a `-light` twin drawn for 32 pixels, which is what a row uses
   * (see taskStateIcon). These are for the menu, at 20.
   */
  readonly statusIcon = (status: TaskStatusLabel): string => {
    const map: Record<TaskStatusLabel, string> = {
      'To do': 'to-do',
      Doing: 'doing',
      Done: 'done',
    };
    return map[status];
  };

  /** Stuck on something that is not a person, and not finished. */
  private readonly isBlocked = (task: TaskData): boolean =>
    task.status !== 'Done' && task.blockedReason !== null;

  /**
   * The icon in front of a row says how far the work has got, and nothing else.
   * A task waiting on somebody is a to-do that happens to be theirs, and one
   * they have started is under way whoever is doing it, so both keep the icon of
   * the state they are in. Who is waited on is what the line under the title is
   * for; saying it twice, in two channels, is how the two started to disagree.
   *
   * The exception is a task stuck on something that is not a person. There the
   * state it is in says nothing useful — it is not moving at all — so it gets
   * the barred circle instead. In the light weight, because a row draws these
   * at 32.
   */
  readonly taskStateIcon = (task: TaskData): string =>
    this.isBlocked(task) ? 'blocked-light' : `${this.statusIcon(task.status)}-light`;

  /**
   * The state line as a row shows it: nothing at all when the line would say no
   * more than the icon in front of it. To do, Doing and Done are drawn there
   * already, so writing them underneath adds a line without adding a fact.
   *
   * What stays is everything the icon cannot draw: who has it, that nobody has,
   * that it is stuck and why, and who finished it.
   */
  rowStateLine(task: TaskData): string | null {
    const line = this.stateLine(task);
    return line === task.status ? null : line;
  }

  /**
   * The tags a row shows: where the work is, then what kind of work it is. The
   * place is stored as one string ("AMS1 · R02-2"), a site and a rack, so it
   * reads as two tags rather than one long one. It sat above the title as an
   * overline before, which said the same thing in a second shape: a rack is a
   * property of the task like every other tag.
   */
  readonly tagCloud = (task: TaskData): string[] => {
    const place = task.location
      .split('·')
      .map((part) => part.trim())
      .filter(Boolean);
    return [...place, ...task.tags];
  };

  /** The tags in use, in the order the menu shows them. Derived rather than
   *  fixed, so a tag somebody adds turns up here on its own. */
  readonly taskTags = computed(() =>
    [...new Set(this.tasks().flatMap((t) => t.tags))].sort((a, b) => a.localeCompare(b)),
  );

  private readonly dateLocale = 'en-US';

  searchQuery = signal('');

  private readonly route = inject(ActivatedRoute);

  private readonly router = inject(Router);

  private readonly viewParams = toSignal(this.route.paramMap, {
    initialValue: convertToParamMap({}),
  });

  /**
   * Whether the address names a view. The section's own path (/tasks) means you
   * have opened the section and picked nothing yet, and then the pane beside
   * the menu says so rather than showing a list you did not ask for.
   */
  readonly hasSelection = computed(() => this.viewParams().get('view') !== null);

  /**
   * What the menu points at, read from the address. One choice, not three: the
   * menu is navigation, so picking a priority takes you to the priorities the
   * way a link takes you to a page, instead of narrowing what a status already
   * narrowed. Combining comes later, above the list, where a filter belongs.
   *
   * The address is where it lives rather than a signal, so a view can be linked
   * to, opened in a second tab and reached with the browser's back button. A
   * value that names nothing (a status that no longer exists, a typo) falls back
   * to showing everything of that kind rather than an empty list with no way out.
   */
  readonly menuSelection = computed<{ kind: MenuKind; value: string }>(() => {
    const params = this.viewParams();
    const view = params.get('view');
    const value = params.get('value') ?? '';
    switch (view) {
      case 'inbox':
      case 'today':
      case 'waiting':
        return { kind: view, value: 'all' };
      case 'status':
        return {
          kind: 'status',
          value: this.kanbanColumns.find((s) => viewSlug(s) === value) ?? 'all',
        };
      case 'priority':
        return {
          kind: 'priority',
          value: this.menuPriorities.find((p) => viewSlug(p) === value) ?? 'all',
        };
      case 'tag':
        return { kind: 'tag', value };
      default:
        return { kind: 'all', value: 'all' };
    }
  });

  /**
   * The title of the page is the row you picked in the menu. The section name
   * is already in the menu's own heading and in the way back, so repeating it
   * above the list would say "Tasks" three times and never say which tasks you
   * are looking at.
   */
  readonly viewTitle = computed(() => {
    const { kind, value } = this.menuSelection();
    switch (kind) {
      case 'inbox':
        return 'Inbox';
      case 'today':
        return 'Today';
      case 'waiting':
        return 'Waiting';
      case 'status':
      case 'priority':
        return value === 'all' ? 'All' : value;
      case 'tag':
        return value;
      default:
        return 'All';
    }
  });

  /** The address of a view, so every row in the menu is a real link. */
  readonly viewPath = (kind: MenuKind, value = ''): string => {
    if (kind === 'all') return `${TASKS_PATH}/all`;
    if (kind === 'status' || kind === 'priority') return `${TASKS_PATH}/${kind}/${viewSlug(value)}`;
    if (kind === 'tag') return `${TASKS_PATH}/tag/${encodeURIComponent(value)}`;
    return `${TASKS_PATH}/${kind}`;
  };

  /**
   * Routes a click in-app while the row stays a real `<a href>`, so middle-click
   * and "open in new tab" keep working. Anything with a modifier is left to the
   * browser. The same trade the shell makes for the sections.
   */
  goToView(event: Event, kind: MenuKind, value = ''): void {
    if (event instanceof MouseEvent) {
      if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0) {
        return;
      }
    }

    event.preventDefault();
    this.router.navigateByUrl(this.viewPath(kind, value));
  }

  /**
   * Back from the list is back to the menu, so the address says so too. Leave it
   * naming a view and the panes and the URL disagree: picking that same view
   * again would be a navigation to where you already are, which does nothing,
   * and the list would stay out of reach.
   */
  goToMenu(): void {
    this.router.navigateByUrl(TASKS_PATH);
  }

  private readonly selectionOf = (kind: 'status' | 'priority' | 'tag') =>
    computed(() => (this.menuSelection().kind === kind ? this.menuSelection().value : 'all'));

  readonly statusFilter = this.selectionOf('status');

  readonly priorityFilter = this.selectionOf('priority');

  readonly tagFilter = this.selectionOf('tag');

  /** Whether this menu row is the one the list is showing. */
  isMenuSelection(kind: MenuKind, value = 'all'): boolean {
    if (!this.hasSelection()) return false;
    return this.menuSelection().kind === kind && this.menuSelection().value === value;
  }

  selectedTasks = signal<Set<string>>(new Set());

  /**
   * The state a click on the icon moves a task to: not started, under way,
   * finished, and round again. The way back is the long way, which is the price
   * of one click instead of a menu; the way forward is the one people take all
   * day.
   */
  readonly nextStatus = (status: TaskStatusLabel): TaskStatusLabel => {
    const order: TaskStatusLabel[] = ['To do', 'Doing', 'Done'];
    return order[(order.indexOf(status) + 1) % order.length];
  };

  /**
   * Whether the list is picking tasks rather than opening them. Off by default:
   * a checkbox on every row is permanent furniture for something you do rarely,
   * and it leaves the front of the row saying two things at once. In this mode
   * the row itself is the checkbox, so a row has one job either way.
   */
  readonly selectionMode = signal(false);

  setSelectionMode(on: boolean): void {
    this.selectionMode.set(on);
    if (!on) this.selectedTasks.set(new Set());
  }

  detailTaskId = signal<string | null>(null);

  // null means "creating a new task"; a string is the id being edited.
  editingTaskId = signal<string | null>(null);

  editFormTitle = signal('');

  editFormDescription = signal('');

  editFormStatus = signal<TaskStatusLabel>('To do');

  editFormPriority = signal<TaskPriorityLabel>('Medium');

  editFormTags = signal<string[]>([]);

  editFormDue = signal('');

  editFormLocation = signal('');

  editFormAssignee = signal<string | null>(null);

  editTitleTouched = signal(false);

  editTitleInvalid = computed(() => this.editTitleTouched() && !this.editFormTitle().trim());

  newNoteText = signal('');

  /** The tasks that belong in this view right now. */
  private readonly matchingTasks = computed(() => {
    const q = this.searchQuery().toLowerCase().trim();
    const st = this.statusFilter();
    const pr = this.priorityFilter();
    const cat = this.tagFilter();
    const view = this.menuSelection().kind;
    return this.tasks().filter((t) => {
      if (st !== 'all' && !this.inStatusView(t, st as TaskStatusLabel)) return false;
      if (pr !== 'all' && t.priority !== pr) return false;
      if (cat !== 'all' && !t.tags.includes(cat)) return false;
      if (view === 'inbox' && !this.isInbox(t)) return false;
      if (view === 'today' && !this.isToday(t)) return false;
      if (view === 'waiting' && !this.isWaiting(t)) return false;
      if (q && !t.title.toLowerCase().includes(q) && !t.description.toLowerCase().includes(q))
        return false;
      return true;
    });
  });

  /**
   * Ids that stopped matching while you were looking at them. Set a task in the
   * To do view to Doing and it no longer belongs there, but taking it away under
   * the pointer moves every row below it up, and the next click lands on
   * something else. So it stays until you leave the view, which is the one moment
   * you choose yourself: no timer, nothing creeping away mid-gesture. That the
   * change landed is visible anyway — the icon changes, and the counts in the
   * menu go with it.
   */
  private readonly heldIds = signal<Set<string>>(new Set());

  private readonly seenView = signal('');

  private readonly seenIds = signal<Set<string>>(new Set());

  /** What the list shows: what belongs here, plus what is being held. */
  readonly filteredTasks = computed(() => {
    const matching = this.matchingTasks();
    const held = this.heldIds();
    const shown =
      held.size === 0
        ? matching
        : [...matching, ...this.tasks().filter((t) => held.has(t.id) && !matching.includes(t))];

    // Soonest first, and everything without a date after it: a task with no due
    // date has no claim on today, so it cannot outrank one that has. The sort is
    // stable, so within a date the order the server sent (newest first) stands.
    return [...shown].sort((a, b) => {
      if (a.due === b.due) return 0;
      if (!a.due) return 1;
      if (!b.due) return -1;
      return a.due < b.due ? -1 : 1;
    });
  });

  statusCounts = computed(() =>
    this.kanbanColumns.reduce<Record<string, number>>(
      (counts, status) => ({
        ...counts,
        [status]: this.tasks().filter((t) => this.inStatusView(t, status)).length,
      }),
      {},
    ),
  );

  priorityCounts = computed(() =>
    this.tasks().reduce<Record<string, number>>(
      (acc, t) => ({ ...acc, [t.priority]: (acc[t.priority] ?? 0) + 1 }),
      {},
    ),
  );

  tagCounts = computed(() =>
    this.tasks().reduce<Record<string, number>>((acc, t) => {
      t.tags.forEach((tag) => {
        acc[tag] = (acc[tag] ?? 0) + 1;
      });
      return acc;
    }, {}),
  );

  // Drives the header checkbox. Mirrors toggleSelectAll's scope (the filtered
  // rows) so the control reflects what it actually selects.
  allFilteredSelected = computed(() => {
    const filtered = this.filteredTasks();
    if (filtered.length === 0) return false;
    const selected = this.selectedTasks();
    return filtered.every((t) => selected.has(t.id));
  });

  /** Some but not all: the select-all box then shows the in-between state. */
  readonly someFilteredSelected = computed(() => {
    const selected = this.selectedTasks();
    return !this.allFilteredSelected() && this.filteredTasks().some((t) => selected.has(t.id));
  });

  /** How much this view holds, above the list where the list is counted. */
  readonly listSummary = computed(() => {
    const total = this.filteredTasks().length;
    return `${total} task${total === 1 ? '' : 's'}`;
  });

  /**
   * What the bar at the bottom acts on. It says so itself rather than leaning on
   * the count at the top of the list, because once you have scrolled the bar is
   * the only part of the two you can still see.
   */
  readonly selectionSummary = computed(() => {
    const selected = this.selectedTasks().size;
    return selected === 0 ? 'Nothing selected' : `${selected} selected`;
  });

  detailTask = computed(() => {
    const id = this.detailTaskId();
    if (id === null) return null;
    return this.tasks().find((t) => t.id === id) ?? null;
  });

  editModalTitle = computed(() => (this.editingTaskId() ? 'Edit task' : 'New task'));

  readonly kanbanSheetEl = viewChild<ElementRef<NlddSheet>>('kanbanSheetEl');

  readonly detailSheetEl = viewChild<ElementRef<NlddSheet>>('detailSheetEl');

  readonly editModalEl = viewChild<ElementRef<NlddSheet>>('editModalEl');

  readonly editTitleInput = viewChild<ElementRef<HTMLElement>>('editTitleInput');

  readonly deleteDialogEl = viewChild<ElementRef<NlddSheet>>('deleteDialogEl');

  readonly bulkDeleteDialogEl = viewChild<ElementRef<NlddSheet>>('bulkDeleteDialogEl');

  readonly bulkStatusPopoverEl = viewChild<ElementRef<NlddSheet>>('bulkStatusPopoverEl');

  readonly bulkAssignPopoverEl = viewChild<ElementRef<NlddSheet>>('bulkAssignPopoverEl');

  getTech(id: string | null): Technician | null {
    return this.technicians().find((t) => t.id === id) ?? null;
  }

  formatDate(str: string | null): string {
    if (!str) return '—';
    const d = new Date(`${str}T00:00:00`);
    return d.toLocaleDateString(this.dateLocale, {
      month: 'short',
      day: 'numeric',
      year: 'numeric',
    });
  }

  // Uses the trailing (random) hex of the uuid rather than the leading bytes,
  // which in uuidv7 are a millisecond timestamp and collide across tasks
  // created close together.
  readonly taskDisplayId = (task: Task): string =>
    `T-${task.id.replace(/-/g, '').slice(-8).toUpperCase()}`;

  isSelected(id: string): boolean {
    return this.selectedTasks().has(id);
  }

  statusStyle(status: string): StatusStyle {
    return this.statusStyles[status] ?? this.statusStyles['Ready'];
  }

  priorityStyle(priority: string): PriorityStyle {
    return this.priorityStyles[priority] ?? this.priorityStyles['Medium'];
  }

  // One pass over the filtered tasks per change, rather than one filter per call
  // site. The template reads each column three times (count, [cdkDropListData],
  // @for), so a plain method handed CDK a freshly allocated array — a new
  // identity for the same contents — on every change-detection cycle.
  private readonly tasksByColumn = computed(() => {
    const byColumn = new Map<TaskStatusLabel, Task[]>(this.kanbanColumns.map((c) => [c, []]));
    this.filteredTasks().forEach((task) => byColumn.get(task.status)?.push(task));
    return byColumn;
  });

  tasksForColumn(col: TaskStatusLabel): Task[] {
    return this.tasksByColumn().get(col) ?? [];
  }

  onKanbanDrop(event: CdkDragDrop<Task[]>, targetStatus: TaskStatusLabel): void {
    const task = event.item.data as Task;
    if (!task) return;
    this.setStatus(task, targetStatus);
  }

  /**
   * Moves one task to another status, from the menu on its icon or from a drop
   * on a kanban lane. The row changes first and rolls back if the write fails,
   * because a status you set by clicking should land under the pointer rather
   * than a round trip later.
   *
   * Only the status is sent. Sending the row's whole snapshot would write back
   * every field as this board last saw it, silently reverting any edit another
   * admin made since it loaded.
   */
  setStatus(task: TaskData, status: TaskStatusLabel): void {
    if (task.status === status) return;

    const previousStatus = task.status;
    this.tasks.update((list) => list.map((t) => (t.id === task.id ? { ...t, status } : t)));

    firstValueFrom(this.taskApi.updateTask(task.id, { status }))
      .then(() => this.loadTasks())
      .catch((err) => {
        // eslint-disable-next-line no-console
        console.error(connectErrorMessage(err));
        this.toast.show('Could not move task');
        this.tasks.update((list) =>
          list.map((t) => (t.id === task.id ? { ...t, status: previousStatus } : t)),
        );
      });
  }

  openKanban(): void {
    this.kanbanSheetEl()?.nativeElement.show();
  }

  closeKanban(): void {
    this.kanbanSheetEl()?.nativeElement.hide();
  }

  toggleSelection(id: string, checked: boolean): void {
    this.selectedTasks.update((set) => {
      const next = new Set(set);
      if (checked) next.add(id);
      else next.delete(id);
      return next;
    });
  }

  // Scoped to the filtered rows, not the whole board: the checkbox sits in the
  // header of a table that renders filteredTasks(), so selecting the full task
  // list would hand bulk delete/update tasks the user cannot see.
  toggleSelectAll(checked: boolean): void {
    if (checked) {
      this.selectedTasks.set(new Set(this.filteredTasks().map((t) => t.id)));
    } else {
      this.selectedTasks.set(new Set());
    }
  }

  openDetail(id: string): void {
    this.detailTaskId.set(id);
    // Always refetched, since another admin may have added a note since this
    // task was last opened. Anything already cached stays on screen meanwhile,
    // so the list does not flash empty while the response is in flight.
    this.loadNotes(id);
    this.detailSheetEl()?.nativeElement.show();
  }

  private loadNotes(id: string): void {
    this.notesError.set(null);
    firstValueFrom(this.noteApi.listNotesForTask(id))
      .then((res) => {
        const notes = res.notes.map((n) => TasksComponent.mapNote(n));
        this.tasks.update((tasks) => tasks.map((t) => (t.id === id ? { ...t, notes } : t)));
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
    const id = this.detailTaskId();
    if (id !== null) this.loadNotes(id);
  }

  private static mapNote(n: ProtoNote): Note {
    return {
      author: n.createdBy,
      authorId: n.createdById ? n.createdById : null,
      text: n.body,
      time: TasksComponent.relativeTime(n.created ? timestampDate(n.created) : new Date()),
    };
  }

  private static relativeTime(date: Date): string {
    const diffMs = Date.now() - date.getTime();
    const days = Math.floor(diffMs / 86_400_000);
    if (days <= 0) return 'Today';
    if (days === 1) return '1 day ago';
    return `${days} days ago`;
  }

  closeDetail(): void {
    this.detailSheetEl()?.nativeElement.hide();
    this.detailTaskId.set(null);
  }

  openEditFromDetail(): void {
    const id = this.detailTaskId();
    this.detailSheetEl()?.nativeElement.hide();
    this.openEditModal(id);
  }

  openDeleteDialog(): void {
    this.deleteDialogEl()?.nativeElement.show();
  }

  closeDeleteDialog(): void {
    this.deleteDialogEl()?.nativeElement.hide();
  }

  confirmDeleteTask(): void {
    const id = this.detailTaskId();
    this.closeDeleteDialog();
    if (id === null) return;
    firstValueFrom(this.taskApi.deleteTask(id))
      .then(() => {
        this.closeDetail();
        this.loadTasks();
        this.toast.show('Task deleted');
      })
      .catch((err) => {
        // eslint-disable-next-line no-console
        console.error(connectErrorMessage(err));
        this.toast.show('Could not delete task');
      });
  }

  openBulkDeleteDialog(): void {
    this.bulkDeleteDialogEl()?.nativeElement.show();
  }

  closeBulkDeleteDialog(): void {
    this.bulkDeleteDialogEl()?.nativeElement.hide();
  }

  confirmBulkDelete(): void {
    // Same scoping as bulkUpdate: a selection can outlive the task it points at
    // (another admin deleted it), and those ids would only produce noise.
    const ids = this.selectedExistingTaskIds();
    this.closeBulkDeleteDialog();
    if (ids.length === 0) return;
    settledPool(ids, BULK_CONCURRENCY, (id) => firstValueFrom(this.taskApi.deleteTask(id)))
      .then((results) => {
        this.selectedTasks.set(new Set());
        this.loadTasks();
        // The pool settles rather than rejects, so surface partial failures
        // explicitly rather than reporting blanket success.
        const failed = TasksComponent.countRejections(results);
        this.toast.show(
          failed === 0
            ? `${ids.length} task(s) deleted`
            : `${ids.length - failed} of ${ids.length} deleted, ${failed} failed`,
        );
      })
      .catch((err) => {
        // settledPool itself never rejects, so this only fires if the handler
        // above throws — which would otherwise surface as an unhandled rejection.
        // eslint-disable-next-line no-console
        console.error(connectErrorMessage(err));
      });
  }

  // The selected ids that still correspond to a loaded task. Membership goes
  // through a Set rather than a nested scan: select-all on a large board makes
  // both collections the size of the board, so a .some() per id is quadratic.
  private selectedExistingTaskIds(): string[] {
    const loaded = new Set(this.tasks().map((t) => t.id));
    return [...this.selectedTasks()].filter((id) => loaded.has(id));
  }

  bulkSetStatus(status: TaskStatusLabel): void {
    this.bulkStatusPopoverEl()?.nativeElement.hide();
    this.bulkUpdate({ status }, 'Status updated');
  }

  // assignee of null unassigns the selected tasks (the "Unassigned" option).
  bulkAssign(assigneeId: string | null): void {
    this.bulkAssignPopoverEl()?.nativeElement.hide();
    this.bulkUpdate({ assignee: assigneeId }, assigneeId ? 'Tasks reassigned' : 'Tasks unassigned');
  }

  // Applies `patch` to every selected task. Like the kanban drop, this sends
  // only the changed fields — never the board's snapshot of the other ones.
  private bulkUpdate(patch: TaskPatch, successMessage: string): void {
    const ids = this.selectedExistingTaskIds();
    if (ids.length === 0) return;
    settledPool(ids, BULK_CONCURRENCY, (id) => firstValueFrom(this.taskApi.updateTask(id, patch)))
      .then((results) => {
        this.loadTasks();
        // The pool settles rather than rejects, so surface partial failures
        // explicitly rather than reporting blanket success.
        const failed = TasksComponent.countRejections(results);
        this.toast.show(
          failed === 0
            ? successMessage
            : `${ids.length - failed} of ${ids.length} updated, ${failed} failed`,
        );
      })
      .catch((err) => {
        // eslint-disable-next-line no-console
        console.error(connectErrorMessage(err));
      });
  }

  // Counts rejected settlements and logs their reasons, for the bulk operations
  // that use settledPool (which resolves even when individual calls fail).
  private static countRejections(results: PromiseSettledResult<unknown>[]): number {
    const rejected = results.filter((r): r is PromiseRejectedResult => r.status === 'rejected');
    // eslint-disable-next-line no-console
    rejected.forEach((r) => console.error(connectErrorMessage(r.reason)));
    return rejected.length;
  }

  openEditModal(taskId: string | null): void {
    this.editingTaskId.set(taskId);
    const task = taskId !== null ? this.tasks().find((t) => t.id === taskId) : null;
    this.editFormTitle.set(task?.title ?? '');
    this.editFormDescription.set(task?.description ?? '');
    this.editFormStatus.set(task?.status ?? 'To do');
    this.editFormPriority.set(task?.priority ?? 'Medium');
    this.editFormTags.set(task ? [...task.tags] : []);
    this.editFormDue.set(task?.due ?? '');
    this.editFormLocation.set(task?.location ?? '');
    this.editFormAssignee.set(task?.assignee ?? null);
    this.editTitleTouched.set(false);
    this.editModalEl()?.nativeElement.show();
    // setTimeout ensures the sheet and Lit's async shadow DOM render have
    // completed before calling focus(), since Lit renders on microtasks.
    setTimeout(() => this.editTitleInput()?.nativeElement.focus());
  }

  closeEditModal(): void {
    this.editModalEl()?.nativeElement.hide();
    this.editingTaskId.set(null);
  }

  saveTask(): void {
    this.editTitleTouched.set(true);
    const title = this.editFormTitle().trim();
    if (!title) return;

    const input: TaskInput = {
      title,
      description: this.editFormDescription().trim(),
      status: this.editFormStatus(),
      priority: this.editFormPriority(),
      tags: this.editFormTags(),
      due: this.editFormDue(),
      location: this.editFormLocation().trim(),
      assignee: this.editFormAssignee(),
    };

    const editingId = this.editingTaskId();

    let request: Observable<unknown>;
    if (editingId === null) {
      request = this.taskApi.createTask(input);
    } else {
      // Only the fields this form actually changed are sent, for the same
      // reason the kanban drop and the bulk actions send a single key: posting
      // the whole form writes back every column as this board last saw it, so
      // an edit here would silently revert a status another admin changed
      // since the sheet was opened.
      const current = this.tasks().find((t) => t.id === editingId);
      const patch = current ? TaskApiService.changedFields(current, input) : input;
      if (Object.keys(patch).length === 0) {
        this.closeEditModal();
        return;
      }
      request = this.taskApi.updateTask(editingId, patch);
    }

    firstValueFrom(request)
      .then(() => {
        this.loadTasks();
        this.toast.show(editingId !== null ? 'Task updated' : 'Task created');
        this.closeEditModal();
      })
      .catch((err) => {
        // eslint-disable-next-line no-console
        console.error(connectErrorMessage(err));
        this.toast.show('Could not save task');
      });
  }

  addNote(): void {
    const text = this.newNoteText().trim();
    if (!text) return;
    const id = this.detailTaskId();
    if (id === null) return;
    firstValueFrom(this.noteApi.createNoteForTask(id, text))
      .then(() => {
        this.newNoteText.set('');
        this.loadNotes(id);
        this.toast.show('Note added');
      })
      .catch((err) => {
        // eslint-disable-next-line no-console
        console.error(connectErrorMessage(err));
        this.toast.show('Could not add note');
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
    const tech = note.authorId ? this.getTech(note.authorId) : null;
    return { name: note.author || 'Unknown', tech };
  }

  // True when a task carries an assignee that the roster has no entry for —
  // typically someone soft-deleted since the board loaded. Rendering that as
  // "Unassigned" would quietly drop the fact that the task is still spoken for.
  //
  // Gated on the roster having actually loaded. ListUsers runs concurrently
  // with ListTasks, so without the guard every assigned task claims its
  // assignee has left for as long as the roster is in flight — and permanently
  // if ListUsers fails, which is a statement about live data the board is in no
  // position to make.
  assigneeMissing(task: TaskData): boolean {
    return this.rosterLoaded() && task.assignee !== null && this.getTech(task.assignee) === null;
  }

  // True when a task has an assignee the board cannot name yet, because the
  // roster is still loading or failed to load. Distinct from unassigned: the
  // task is spoken for, we just cannot say by whom.
  assigneeUnresolved(task: TaskData): boolean {
    return !this.rosterLoaded() && task.assignee !== null;
  }

  // Goes through the same closers as the buttons: hiding the sheets without
  // clearing detailTaskId/editingTaskId would leave a "closed" task still
  // addressable by addNote() and detailTask().
  onEscape(): void {
    this.closeDetail();
    this.closeEditModal();
  }

  statusTagColor(status: string): string {
    return this.statusStyle(status).tagColor;
  }

  statusDotClass(status: string): string {
    return `h-1.5 w-1.5 rounded-full ${this.statusStyle(status).dot} shrink-0`;
  }

  priorityTagColor(priority: string): string {
    return this.priorityStyle(priority).tagColor;
  }

  kanbanCardClass(status: string): string {
    const s = this.statusStyle(status);
    return `cursor-grab active:cursor-grabbing rounded-xl border ${s.kanbanBorder} bg-white dark:bg-gray-950 p-3.5 hover:shadow-md hover:shadow-slate-200/80 dark:hover:shadow-black/60 transition-shadow`;
  }

  readonly techInitialsClass = (tech: Technician, size = 'h-7 w-7 text-xs'): string =>
    `inline-flex ${size} items-center justify-center rounded-full ${tech.color} text-white font-semibold shrink-0`;

  readonly unassignedAvatarClass = (size = 'h-7 w-7 text-xs'): string =>
    `inline-flex ${size} items-center justify-center rounded-full bg-slate-200 dark:bg-gray-800 text-slate-500 dark:text-gray-400 font-medium shrink-0`;

  // Deliberately not the unassigned grey: an assignee who has left the roster is
  // a different state from nobody being assigned, and reads as one at a glance.
  readonly unknownAvatarClass = (size = 'h-7 w-7 text-xs'): string =>
    `inline-flex ${size} items-center justify-center rounded-full bg-amber-100 dark:bg-amber-950 text-amber-700 dark:text-amber-300 font-semibold shrink-0`;

  shortDate(str: string | null): string {
    if (!str) return '';
    return new Date(`${str}T00:00:00`).toLocaleDateString(this.dateLocale, {
      month: 'short',
      day: 'numeric',
    });
  }
}
