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
import { firstValueFrom } from 'rxjs';
import {
  CdkDropList,
  CdkDropListGroup,
  CdkDrag,
  CdkDragPlaceholder,
  CdkDragDrop,
} from '@angular/cdk/drag-drop';
import TaskStore, { Task, Technician } from '../task-management/task-store';
import TaskStatusUi from '../task-management/task-status-ui.service';
import TaskStatusDialogsComponent from '../task-management/task-status-dialogs/task-status-dialogs';
import TaskDetailComponent from './task-detail/task-detail';
import TaskApiService, {
  TaskData,
  TaskPatch,
  TaskPriorityLabel,
  TaskStatusLabel,
} from '../task-management/task-api.service';
import UserApiService from '../task-management/user-api.service';
import NoteApiService from '../inventory/note-api.service';
import settledPool from '../shared/settled-pool';
import ToastService from '../shared/toast.service';
import connectErrorMessage from '../../connect/error';
import SecondaryNavService from '../shell/secondary-nav.service';
import { TASKS_PATH, viewSlug } from './task-views';
import openOnCreateRequest from '../shell/create-request';
import OverlayService from '../shell/overlay.service';
import TaskAttentionService from './task-attention.service';
import DatacenterListService from '../datacenters/datacenter-list.service';
import PlacementApiService from '../inventory/placement-api.service';
import { buildTagTree, tagMatches, taskTags } from './task-tags';
import { Round, taskDatacenter } from '../rounds/round';
import RoundsService from '../rounds/rounds.service';
import TaskManagementTechnicianComponent from '../task-management-technician/task-management-technician';

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

// Ceiling on in-flight requests for the bulk actions. Select-all over a large
// board would otherwise put one request per task on the wire at once.
const BULK_CONCURRENCY = 6;

@Component({
  selector: 'app-tasks',
  templateUrl: './tasks.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [
    NgTemplateOutlet,
    CdkDropListGroup,
    CdkDropList,
    CdkDrag,
    CdkDragPlaceholder,
    TaskStatusDialogsComponent,
    TaskDetailComponent,
    TaskManagementTechnicianComponent,
  ],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  // The split view hides the back button of every bar in the main pane, because
  // the menu beside you is the way back. A sheet has no menu beside it, so a bar
  // in there keeps its own. Drop this once the design system ships the same
  // reset on nldd-sheet.
  styles: `
    nldd-sheet {
      --context-back-button-display: flex;
    }
  `,
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

  private readonly store = inject(TaskStore);

  protected readonly ui = inject(TaskStatusUi);

  private readonly taskApi = inject(TaskApiService);

  /** The task form lives in the shell, so it can be opened from anywhere. */
  private readonly overlays = inject(OverlayService);

  /** Says when a task changed somewhere else than this page. */
  private readonly attention = inject(TaskAttentionService);

  /** The data centers, so the menu can list them as the fixed tags they are. */
  private readonly datacenterList = inject(DatacenterListService);

  /** Which round a task sits in, for the box at the bottom of its detail. */
  private readonly roundsService = inject(RoundsService);

  private readonly datacenters = this.datacenterList.datacenters;

  private readonly userApi = inject(UserApiService);

  private readonly noteApi = inject(NoteApiService);

  protected readonly toast = inject(ToastService);

  readonly technicians = this.store.technicians;

  // False until ListUsers has actually answered. An empty roster because the
  // response has not landed (or failed) is not the same fact as an assignee
  // having left the directory, and assigneeMissing() must not report the first
  // as the second — see assigneeUnresolved().
  readonly rosterLoaded = signal(false);

  /** One list, shared with the sheet the shell opens: see TaskStore. */
  readonly tasks = this.store.tasks;

  // Set when the board could not be loaded, so an API failure reads as an error
  // rather than as a legitimately empty board.
  readonly loadError = this.store.loadError;


  // The signed-in user's roster entry, for the note composer's avatar. Null when
  // the caller has no directory entry — GetCurrentUser answers NotFound then,
  // which is an ordinary provisioning state and not worth a toast here: the
  // board still works, and any note they write comes out unattributed.
  readonly currentUser = this.store.currentUser;

  constructor() {
    // The halls are fixed entries in the tag menu, so this page needs them
    // whether or not a task mentions one.
    this.datacenterList.load();
    // A task made from the add button in the bar is saved somewhere else, so
    // the list reads itself again when the shell says something changed.
    effect(() => {
      this.attention.changed();
      untracked(() => this.loadTasks());
    });
    // The add menu in the bar asks for this form through the address.
    openOnCreateRequest(() => this.openNewTask());
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
    this.loadCurrentUser();
    this.loadUsers();
    this.loadTasks();
  }

  ngOnDestroy(): void {
    this.secondaryNav.clear(this.secondaryNavTemplate());
  }

  private loadCurrentUser(): void {
    firstValueFrom(this.userApi.getCurrentUser())
      .then((res) => this.store.currentUser.set(res.user ? UserApiService.mapUser(res.user) : null))
      .catch((err) => {
        // eslint-disable-next-line no-console
        console.error(connectErrorMessage(err));
        this.store.currentUser.set(null);
      });
  }

  private loadUsers(): void {
    firstValueFrom(this.userApi.listUsers())
      .then((res) => {
        this.store.technicians.set(res.users.map((u) => UserApiService.mapUser(u)));
        this.rosterLoaded.set(true);
      })
      .catch((err) => {
        // eslint-disable-next-line no-console
        console.error(connectErrorMessage(err));
        // Left false deliberately: without a roster the board cannot tell a
        // departed assignee from one it merely failed to look up, so it says
        // neither rather than picking the alarming reading.
        this.rosterLoaded.set(false);
        this.toast.error('Could not load the technician roster');
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
    this.store.loadTasks();
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
    // Three questions, and a task missing any of them cannot enter a round:
    // who walks it, when, and where. A place is a tag like any other, so a task
    // with no tag naming a data center has nowhere to be walked.
    if (!task.assignee || task.due === '') return true;
    return !taskDatacenter(
      task,
      this.datacenterList.datacenters().map((dc) => dc.name),
    );
  };

  readonly inboxCount = computed(() => this.tasks().filter((t) => this.isInbox(t)).length);

  /**
   * What is not yours to move. Somebody else's to-do is your waiting-for, and a
   * task stuck on a part rather than a person waits just as hard. Same rule as
   * the line under the title, so the view and the row can never disagree.
   */
  isWaiting(task: TaskData): boolean {
    return this.store.isWaiting(task);
  }

  readonly waitingCount = computed(() => this.tasks().filter((t) => this.isWaiting(t)).length);

  /**
   * Whether a task is yours. An unknown current user is not the same fact as
   * "assigned to somebody else": the answer is unavailable, so nothing is
   * filtered out on it and you see a full list rather than a convincing empty
   * one.
   */
  private isMine(task: TaskData): boolean {
    return this.store.isMine(task);
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
    // Waiting is a state of its own in the menu, so it is one here too: a task
    // held up on a part is not something you are doing, however far along the
    // work itself is.
    return this.isMine(task) && !this.isWaiting(task);
  }

  /**
   * Why a task is not yours to move. Being stuck on something that is not a
   * person is stored on the task; lying with a person is not stored at all but
   * read off the assignee, because it depends on who is looking: the task you
   * are waiting for is the task somebody else has to do. Returns nothing when
   * the work is yours, or finished.
   */
  holdReason(task: TaskData): string | null {
    return this.store.holdReason(task);
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
  readonly priorityIcon = this.store.priorityIcon;

  /**
   * Status as one ring three times over: empty, a second ring inside it, ticked.
   * The three read as one series that way, which a set of unrelated pictures
   * would not. Alias names, because those say what the row is rather than what
   * the icon draws.
   *
   * Every name has a `-light` twin drawn for 32 pixels, which is what a row uses
   * (see taskStateIcon). These are for the menu, at 20.
   */
  readonly statusIcon = this.store.statusIcon;

  /**
   * The icon in front of a row says how far the work has got, and nothing else.
   * A task has three states — to do, doing, done — and being stuck is not a
   * fourth: a blocked task is still to do or still under way, it just is not
   * moving. So it keeps the icon of the state it is in, and the reason it is
   * stuck is the line under the title, where the words are. A fourth icon read
   * as a fourth state and made the round of the tooltip ("Mark as …") say
   * something the icon contradicted. In the light weight, because a row draws
   * these at 32.
   */
  /**
   * The icon a row leads with: what the task is for you.
   *
   * A clock when it is waiting, whatever the stored status says. Somebody else
   * has it, or it is stuck on a thing, and either way it is not yours to pick
   * up or carry on with — which is what To do and Doing would claim. Your own
   * work keeps its status icon, and Done stays Done for everyone.
   */
  readonly taskStateIcon = this.store.taskStateIcon;

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
  readonly tagCloud = (task: TaskData): string[] => taskTags(task);

  /** Everything a task is filed under, as one list of paths. See task-tags.ts. */
  readonly taskTagList = this.store.taskTagList;

  /** The place a task names, as a card shows it: the last step of the first tag
   *  that is a path. A tag without a slash is a word, not a place. */
  readonly placeLabel = (task: TaskData): string => {
    const path = taskTags(task).find((tag) => tag.includes('/'));
    return path ? path.slice(path.lastIndexOf('/') + 1) : '';
  };

  /**
   * The tag menu, as the tree the paths describe: a data center with its racks
   * under it, then the free tags. The data centers are always there, whether
   * or not there is work in them.
   */
  /** Which branches of the tag menu you folded open. Shut is the default: the
   *  menu is a list of places and words, and every rack at once buries the
   *  words underneath them. */
  private readonly expandedTags = signal<ReadonlySet<string>>(new Set());

  /** Open when you opened it, and open when what you are filtering on lives
   *  inside it: a view you cannot see in the menu reads as no view at all. */
  isTagExpanded(path: string): boolean {
    if (this.expandedTags().has(path)) return true;
    const selection = this.menuSelection();
    return (
      this.hasSelection() &&
      selection.kind === 'tag' &&
      String(selection.value).startsWith(`${path}/`)
    );
  }

  /** Folds a branch open or shut. Separate from picking it: the chevron is its
   *  own control, so opening a hall does not also filter on it. */
  toggleTag(event: Event, path: string): void {
    event.stopPropagation();
    event.preventDefault();
    this.expandedTags.update((paths) => {
      const next = new Set(paths);
      if (!next.delete(path)) next.add(path);
      return next;
    });
  }

  readonly tagTree = computed(() =>
    buildTagTree(
      this.tasks().map((task) => taskTags(task)),
      this.datacenters().map((dc) => dc.name),
    ),
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
      if (cat !== 'all' && !taskTags(t).some((tag) => tagMatches(tag, cat))) return false;
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


  readonly kanbanSheetEl = viewChild<ElementRef<NlddSheet>>('kanbanSheetEl');


  readonly technicianSheetEl = viewChild<ElementRef<NlddSheet>>('technicianSheetEl');


  readonly bulkDeleteDialogEl = viewChild<ElementRef<NlddSheet>>('bulkDeleteDialogEl');

  readonly bulkStatusPopoverEl = viewChild<ElementRef<NlddSheet>>('bulkStatusPopoverEl');

  readonly bulkAssignPopoverEl = viewChild<ElementRef<NlddSheet>>('bulkAssignPopoverEl');

  getTech(id: string | null): Technician | null {
    return this.store.getTech(id);
  }

  formatDate(str: string | null): string {
    return this.store.formatDate(str);
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
   * The status, and with it the end of any waiting. You cannot be waiting on
   * something and to-do at the same time, so choosing one of the three lets go
   * of the other; that is why there is no separate way to stop waiting.
   */
  setStatus(task: Task, status: TaskStatusLabel, from?: Event): void {
    this.ui.setStatus(task, status, from);
  }

  /**
   * Writes one field of one task and nothing else.
   *
   * Changing a task is a series of small facts, each true the moment you set
   * it, so there is nothing to submit: the field you touched is the patch.
   * Sending the whole snapshot instead would write back every field as this
   * board last saw it and silently revert what somebody else changed.
   *
   * The new value is on screen before the write comes back, because that is
   * where you just put it. A failed write puts the old one back and says which
   * field it was, in a message that stays: by then the sheet it belonged to may
   * be closed, and one that fades takes the loss with it.
   */
  private patchTask(task: TaskData, patch: TaskPatch, what: string): void {
    this.store.patchTask(task, patch, what);
  }

  private readonly placementApi = inject(PlacementApiService);

  /** Every rack, to offer as a tag under the data center it stands in. */
  private readonly racks = this.store.racks;

  /**
   * What the tag field offers: the places first, then the tags other tasks
   * already carry. A data center and a rack exist whether a task mentions them
   * or not, so they come from their own lists; anything else is free, which is
   * why the two are one list rather than two.
   */
  readonly knownTags = this.store.knownTags;


  /** The name the combo box shows for whoever it is for. */
  assigneeName(task: TaskData): string {
    return this.store.assigneeName(task);
  }


  /** Whose task this is, if not yours. Nobody, or you, both read as yours. */
  somebodyElses(task: TaskData): boolean {
    return this.store.somebodyElses(task);
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

  readonly detailSheetEl = viewChild<ElementRef<NlddSheet>>('detailSheetEl');

  /** Which task the sheet is on. What is in it is app-task-detail. */
  readonly detailTaskId = signal<string | null>(null);

  readonly detailTask = computed(() => {
    const id = this.detailTaskId();
    if (id === null) return null;
    return this.store.tasks().find((t) => t.id === id) ?? null;
  });

  /** Drives the @defer around the technician view: it only loads once asked
   *  for, and it stays loaded after that. */
  readonly technicianOpen = signal(false);

  openTechnicianView(): void {
    this.technicianOpen.set(true);
    this.technicianSheetEl()?.nativeElement.show();
  }

  /** The round this task is walked in, for the technician view below. */
  roundOf(task: TaskData): Round | null {
    return this.roundsService.roundOf().get(task.id) ?? null;
  }

  openDetail(id: string): void {
    this.detailTaskId.set(id);
    // Read when the detail opens rather than with the page: the rounds are only
    // needed for the box at the bottom of that sheet, and the racks for the tag
    // field in it.
    this.roundsService.load();
    this.store.loadRacks();
    this.detailSheetEl()?.nativeElement.show();
  }

  closeDetail(): void {
    this.detailSheetEl()?.nativeElement.hide();
    this.detailTaskId.set(null);
  }







  private loadRacks(): void {
    this.store.loadRacks();
  }


  closeTechnicianView(): void {
    this.technicianSheetEl()?.nativeElement.hide();
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
        if (failed > 0) {
          this.toast.warning(`${ids.length - failed} of ${ids.length} deleted, ${failed} failed`);
        }
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
    this.bulkUpdate({ status });
  }

  // assignee of null unassigns the selected tasks (the "Unassigned" option).
  bulkAssign(assigneeId: string | null): void {
    this.bulkAssignPopoverEl()?.nativeElement.hide();
    this.bulkUpdate({ assignee: assigneeId });
  }

  // Applies `patch` to every selected task. Like the kanban drop, this sends
  // only the changed fields — never the board's snapshot of the other ones.
  private bulkUpdate(patch: TaskPatch): void {
    const ids = this.selectedExistingTaskIds();
    if (ids.length === 0) return;
    settledPool(ids, BULK_CONCURRENCY, (id) => firstValueFrom(this.taskApi.updateTask(id, patch)))
      .then((results) => {
        this.loadTasks();
        // The pool settles rather than rejects, so surface partial failures
        // explicitly rather than reporting blanket success.
        const failed = TasksComponent.countRejections(results);
        if (failed > 0) {
          this.toast.warning(`${ids.length - failed} of ${ids.length} updated, ${failed} failed`);
        }
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

  /** A new task opens the sheet the shell holds: it outlives this page, and the
   *  add button in the bar opens the same one. Editing a task you have open is
   *  a view of the detail sheet instead, so the task stays on screen. */
  openNewTask(): void {
    this.overlays.newTask();
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

  // Goes through the same closer as the button: hiding the sheet without
  // clearing detailTaskId would leave a "closed" task still addressable by
  // addNote() and detailTask(). The task form closes itself, in the shell.
  onEscape(): void {
    this.closeDetail();
    this.overlays.closeTask();
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
