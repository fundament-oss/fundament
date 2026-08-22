import {
  Component,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
  OnInit,
  signal,
  computed,
  effect,
  ChangeDetectorRef,
  ElementRef,
  inject,
  input,
} from '@angular/core';
import { NgTemplateOutlet } from '@angular/common';
import { Router } from '@angular/router';
import { DomSanitizer, SafeHtml } from '@angular/platform-browser';
import { Code, ConnectError } from '@connectrpc/connect';
import { timestampDate } from '@bufbuild/protobuf/wkt';
import { firstValueFrom } from 'rxjs';
import { taskTags } from '../tasks/task-tags';
import { dayLabel, Round } from '../rounds/round';
import RoundsService from '../rounds/rounds.service';

import type { Task as ProtoTask } from '../../generated/v1/task_pb';
import ThemeService from '../theme.service';
import AuthService from '../auth.service';
import TaskApiService, { TaskPriorityLabel } from '../task-management/task-api.service';
import TaskStepApiService from '../task-management/task-step-api.service';
import UserApiService from '../task-management/user-api.service';
import NoteApiService from '../inventory/note-api.service';
import { NoteComment } from '../inventory/inventory';
import settledPool from '../shared/settled-pool';
import ToastService from '../shared/toast.service';
import connectErrorMessage from '../../connect/error';

interface GatherItem {
  // Stable across reloads and re-orderings, unlike the list position: the
  // checklist is rebuilt from the assigned tasks, whose order and membership
  // change between sessions.
  key: string;
  label: string;
  taskFor?: string;
}

interface Step {
  id: string;
  title: string;
  description: string;
  icon: string;
  svg: string;
  /**
   * A task nobody wrote steps for stands in its own walkthrough as one step.
   * Without that, a one-action job like replacing a filter drops out of the
   * round for lack of a checklist, which is exactly the work you walk for.
   * There is no step to write away for it, so finishing it closes the task.
   */
  whole?: boolean;
}

interface Task {
  id: string;
  title: string;
  priority: 'urgent' | 'high' | 'normal';
  tags: string[];
  location: string;
  steps: Step[];
}

type Phase = 'gather' | 'task';

// Ceiling on in-flight ListTaskSteps calls while the walkthrough loads, matching
// the admin board's BULK_CONCURRENCY.
const STEP_FETCH_CONCURRENCY = 6;

/**
 * Shown when the caller holds a valid token but has no dcim.users row. The
 * roster is provisioned out of band, so this is a provisioning gap to report in
 * plain language, not an error to dump an RPC message for.
 */
const NO_DIRECTORY_ENTRY = 'Your account is not in the technician directory';

/**
 * Progress is persisted by id, not by list position. The task list is ordered
 * by creation date and only holds the technician's open tasks, so a task being
 * assigned, finished, or reassigned between sessions shifts every index — and a
 * positional snapshot would resume them inside the wrong task.
 */
interface ProgressSnapshot {
  phase: Phase;
  taskId: string | null;
  stepId: string | null;
  checkedItems: string[];
  gatherCompleted: boolean;
}

@Component({
  selector: 'app-task-management-technician',
  templateUrl: './task-management-technician.html',
  imports: [NgTemplateOutlet],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: {
    /*
     * Inside a page the host is a box nobody asked for: it carried a font and a
     * text color the page already sets, and it stood in the page's own layout.
     * So there it steps aside and its children take its place.
     *
     * Standing on its own it must not: the `nldd-app-view` under it sizes itself
     * against its ancestors, and `display: contents` takes the host's box out of
     * that chain, so the app-view collapsed and its panes clipped what would not
     * fit — nothing scrolled. Left alone the host is `inline`, which is what
     * every other Angular component that holds an app-view is.
     */
    '[style.display]': "embedded() ? 'contents' : null",
  },
})
export default class TaskManagementTechnicianComponent implements OnInit {
  /**
   * Shown inside something that already has a bar of its own, a sheet over the
   * task you were looking at. The view drops its own top navigation there: two
   * bars stacked say the same thing twice, and the way back is the sheet.
   */
  readonly embedded = input(false, { transform: (v: boolean | string) => v !== false && v !== 'false' });

  /**
   * The round to show: one person, one data center, one day. Without it the
   * view falls back to the open work of whoever is logged in, which is what the
   * technician's own address does until they have an environment of their own.
   */
  readonly round = input<Round | null>(null);

  /**
   * Looking rather than walking. A planner reading somebody else's round has no
   * business ticking off work they did not do, so the bar goes and nothing is
   * written, here or in the browser's own store. Stepping through to read is
   * still allowed: that changes what you look at, not what is there.
   */
  readonly readOnly = input(false, { transform: (v: boolean | string) => v !== false && v !== 'false' });

  private sanitizer = inject(DomSanitizer);

  protected readonly theme = inject(ThemeService);

  private readonly auth = inject(AuthService);

  private readonly router = inject(Router);

  private readonly taskApi = inject(TaskApiService);

  private readonly taskStepApi = inject(TaskStepApiService);

  private readonly userApi = inject(UserApiService);

  private readonly noteApi = inject(NoteApiService);

  private readonly toast = inject(ToastService);

  // Light→dark substitutions for the inline step illustrations: paper/background
  // fills darken, dark line/text colors lighten, vivid status accents stay.
  private static readonly SVG_DARK_MAP: Record<string, string> = {
    white: '#0f172a',
    '#ffffff': '#0f172a',
    '#f8fafc': '#0f172a',
    '#f1f5f9': '#1e293b',
    '#e2e8f0': '#334155',
    '#eef2ff': '#1e1b4b',
    '#c7d2fe': '#3730a3',
    '#a5b4fc': '#818cf8',
    '#cbd5e1': '#475569',
    '#94a3b8': '#64748b',
    '#f0fdf4': '#052e16',
    '#dcfce7': '#052e16',
    '#fef2f2': '#450a0a',
    '#fef3c7': '#451a03',
    '#334155': '#cbd5e1',
    '#b45309': '#fbbf24',
  };

  // Static presentation pool for step illustrations. Real steps come from the
  // API (which carries no artwork); each step is paired with an illustration by
  // its index, cycling through this set and falling back to a generic icon.
  private static readonly STEP_ILLUSTRATIONS: { icon: string; svg: string }[] = [
    {
      icon: 'info-circle',
      svg: `<svg viewBox="0 0 120 100" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
        <rect x="10" y="20" width="100" height="60" rx="6" stroke="#e2e8f0" stroke-width="1.5" fill="#f8fafc"/>
        <rect x="18" y="28" width="30" height="44" rx="3" stroke="#cbd5e1" stroke-width="1" fill="white"/>
        <text x="33" y="42" text-anchor="middle" fill="#94a3b8" font-size="7" font-weight="600">Hall A</text>
        <rect x="56" y="28" width="30" height="44" rx="3" stroke="#6366f1" stroke-width="2" fill="#eef2ff"/>
        <text x="71" y="42" text-anchor="middle" fill="#6366f1" font-size="7" font-weight="600">Hall B</text>
        <line x1="62" y1="50" x2="62" y2="68" stroke="#a5b4fc" stroke-width="1" stroke-dasharray="2 2"/>
        <line x1="68" y1="50" x2="68" y2="68" stroke="#a5b4fc" stroke-width="1" stroke-dasharray="2 2"/>
        <circle cx="68" cy="58" r="4" fill="#6366f1"/>
        <circle cx="68" cy="58" r="2" fill="white"/>
        <text x="71" y="54" text-anchor="middle" fill="#4f46e5" font-size="5">Row 12</text>
      </svg>`,
    },
    {
      icon: 'arrow-right',
      svg: `<svg viewBox="0 0 120 100" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
        <rect x="25" y="15" width="35" height="65" rx="4" stroke="#6366f1" stroke-width="2" fill="#eef2ff"/>
        <rect x="30" y="20" width="25" height="55" rx="2" fill="white" stroke="#a5b4fc" stroke-width="1"/>
        <circle cx="50" cy="48" r="2.5" fill="#6366f1"/>
        <rect x="70" y="30" width="22" height="32" rx="4" stroke="#6366f1" stroke-width="2" fill="#eef2ff"/>
        <rect x="74" y="36" width="14" height="8" rx="2" fill="#a5b4fc"/>
        <rect x="74" y="48" width="14" height="8" rx="2" fill="#c7d2fe"/>
        <path d="M60 48 L70 42" stroke="#6366f1" stroke-width="1.5" stroke-dasharray="3 2"/>
      </svg>`,
    },
    {
      icon: 'database',
      svg: `<svg viewBox="0 0 120 100" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
        <rect x="10" y="15" width="18" height="70" rx="2" stroke="#cbd5e1" stroke-width="1" fill="#f8fafc"/>
        <rect x="30" y="15" width="18" height="70" rx="2" stroke="#cbd5e1" stroke-width="1" fill="#f8fafc"/>
        <rect x="50" y="15" width="18" height="70" rx="2" stroke="#cbd5e1" stroke-width="1" fill="#f8fafc"/>
        <rect x="70" y="12" width="22" height="76" rx="3" stroke="#6366f1" stroke-width="2.5" fill="#eef2ff"/>
        <rect x="74" y="20" width="14" height="6" rx="1" fill="#a5b4fc"/>
        <rect x="74" y="30" width="14" height="6" rx="1" fill="#a5b4fc"/>
        <rect x="74" y="40" width="14" height="6" rx="1" fill="#c7d2fe"/>
      </svg>`,
    },
    {
      icon: 'lock-open',
      svg: `<svg viewBox="0 0 120 100" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
        <rect x="35" y="20" width="50" height="55" rx="6" stroke="#6366f1" stroke-width="2" fill="#eef2ff"/>
        <rect x="43" y="35" width="12" height="10" rx="2" fill="white" stroke="#a5b4fc" stroke-width="1"/>
        <rect x="59" y="35" width="12" height="10" rx="2" fill="white" stroke="#a5b4fc" stroke-width="1"/>
        <rect x="43" y="49" width="12" height="10" rx="2" fill="white" stroke="#a5b4fc" stroke-width="1"/>
        <rect x="59" y="49" width="12" height="10" rx="2" fill="white" stroke="#a5b4fc" stroke-width="1"/>
        <circle cx="75" cy="28" r="4" fill="#22c55e"/>
        <path d="M73 28 l2 2 l3-4" stroke="white" stroke-width="1.5" fill="none"/>
      </svg>`,
    },
    {
      icon: 'search',
      svg: `<svg viewBox="0 0 120 100" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
        <rect x="30" y="8" width="60" height="84" rx="4" stroke="#cbd5e1" stroke-width="1.5" fill="white"/>
        <rect x="35" y="14" width="50" height="7" rx="1.5" fill="#f1f5f9"/>
        <rect x="35" y="24" width="50" height="7" rx="1.5" fill="#eef2ff" stroke="#6366f1" stroke-width="1.5"/>
        <rect x="35" y="34" width="50" height="7" rx="1.5" fill="#f1f5f9"/>
        <rect x="35" y="44" width="50" height="7" rx="1.5" fill="#f1f5f9"/>
        <rect x="35" y="54" width="50" height="7" rx="1.5" fill="#f1f5f9"/>
      </svg>`,
    },
    {
      icon: 'cylinder-split-slash',
      svg: `<svg viewBox="0 0 120 100" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
        <rect x="20" y="25" width="60" height="50" rx="4" stroke="#cbd5e1" stroke-width="1.5" fill="white"/>
        <rect x="26" y="31" width="22" height="16" rx="2" stroke="#ef4444" stroke-width="2" fill="#fef2f2" stroke-dasharray="4 2"/>
        <text x="37" y="42" text-anchor="middle" fill="#ef4444" font-size="6" font-weight="600">Bay 3</text>
        <rect x="52" y="31" width="22" height="16" rx="2" stroke="#cbd5e1" stroke-width="1" fill="#f1f5f9"/>
        <rect x="26" y="52" width="22" height="16" rx="2" stroke="#cbd5e1" stroke-width="1" fill="#f1f5f9"/>
        <rect x="52" y="52" width="22" height="16" rx="2" stroke="#cbd5e1" stroke-width="1" fill="#f1f5f9"/>
      </svg>`,
    },
    {
      icon: 'cylinder-split',
      svg: `<svg viewBox="0 0 120 100" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
        <rect x="20" y="25" width="60" height="50" rx="4" stroke="#cbd5e1" stroke-width="1.5" fill="white"/>
        <rect x="26" y="31" width="22" height="16" rx="2" stroke="#22c55e" stroke-width="2" fill="#f0fdf4"/>
        <text x="37" y="42" text-anchor="middle" fill="#22c55e" font-size="6" font-weight="600">Bay 3</text>
        <rect x="52" y="31" width="22" height="16" rx="2" stroke="#cbd5e1" stroke-width="1" fill="#f1f5f9"/>
        <rect x="26" y="52" width="22" height="16" rx="2" stroke="#cbd5e1" stroke-width="1" fill="#f1f5f9"/>
        <circle cx="30" cy="29" r="3" fill="#22c55e"/>
      </svg>`,
    },
    {
      icon: 'check-mark-circle',
      svg: `<svg viewBox="0 0 120 100" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
        <rect x="25" y="15" width="70" height="50" rx="6" stroke="#6366f1" stroke-width="2" fill="#eef2ff"/>
        <rect x="32" y="22" width="56" height="30" rx="3" fill="white" stroke="#a5b4fc" stroke-width="1"/>
        <text x="60" y="34" text-anchor="middle" fill="#6366f1" font-size="5.5" font-weight="500">Status OK</text>
        <rect x="40" y="39" width="40" height="6" rx="3" fill="#dcfce7"/>
        <rect x="40" y="39" width="32" height="6" rx="3" fill="#22c55e"/>
        <circle cx="35" cy="72" r="5" fill="#22c55e"/>
        <path d="M33 72 l2 2 l3-4" stroke="white" stroke-width="1.5" fill="none"/>
      </svg>`,
    },
  ];

  // Generic part label per tag, used to build the gather checklist.
  private static readonly TAG_PARTS: Record<string, string> = {
    hardware: 'Replacement hardware components',
    network: 'Replacement network device',
    cooling: 'Cooling spares & filters',
    power: 'Power components & fuses',
    security: 'Mounting hardware & cabling',
  };

  /** The first tag we know a parts list for. A task tagged both network and
   *  hardware is network work with a box of hardware, and the more specific
   *  label is the useful one. */
  private static partsLabelFor(tags: string[]): string {
    const known = tags.find((tag) => tag in TaskManagementTechnicianComponent.TAG_PARTS);
    return known ? TaskManagementTechnicianComponent.TAG_PARTS[known] : 'Task-specific parts';
  }

  readonly tasks = signal<Task[]>([]);

  // Static generic tools plus per-task parts derived from the fetched tasks.
  readonly gatherItems = computed<GatherItem[]>(() => {
    const tools: GatherItem[] = [
      { key: 'tool:wrist-strap', label: 'Anti-static wrist strap' },
      { key: 'tool:screwdriver', label: 'Phillips-head screwdriver' },
      { key: 'tool:multimeter', label: 'Multimeter' },
    ];
    const parts = this.tasks().map((t) => ({
      key: `part:${t.id}`,
      label: TaskManagementTechnicianComponent.partsLabelFor(t.tags),
      taskFor: `${t.title} — ${TaskManagementTechnicianComponent.lastLocationSegment(t.location)}`,
    }));
    return [...tools, ...parts];
  });

  private static lastLocationSegment(location: string): string {
    const seg = location.split('·')[1]?.trim();
    return seg ?? location;
  }

  constructor() {
    // Land on a round rather than on a menu: the first of mine, which is the
    // one with the most urgent work in it since that is how they are sorted.
    effect(() => {
      const mine = this.myRounds();
      if (!this.round() && !this.chosenRound() && mine.length) this.chosenRound.set(mine[0]);
    });

    // The round can arrive as an input and can change under the same instance
    // when you pick another one, so it is watched rather than read once.
    effect(() => {
      const round = this.activeRound();
      if (round) this.loadRound(round);
    });

    // The notes follow whichever task is open. Read per task rather than all at
    // once: a round holds five of them and you look at one.
    effect(() => {
      const task = this.tasks()[this.currentTaskIndex()];
      this.taskNotes.set([]);
      if (task) this.loadNotes(task.id);
    });

    // Auto-save progress on every change, once restoreProgress() has run —
    // otherwise this would immediately overwrite a saved snapshot with the
    // signals' initial (empty) values before it's been read back.
    //
    // The snapshot is built BEFORE the hydrated() guard on purpose: an effect
    // only tracks the signals it actually reads, so returning early would leave
    // this depending on hydrated() alone and it would never re-run when progress
    // changes. Do not "tidy" the guard up to the top — that silently disables
    // auto-save.
    effect(() => {
      const snapshot: ProgressSnapshot = {
        phase: this.phase(),
        taskId: this.currentTask()?.id ?? null,
        stepId: this.currentTask()?.steps[this.currentStepIndex()]?.id ?? null,
        checkedItems: [...this.checkedItems()],
        gatherCompleted: this.gatherCompleted(),
      };
      if (!this.hydrated()) return;
      const key = this.storageKey();
      if (key) localStorage.setItem(key, JSON.stringify(snapshot));
    });
  }

  ngOnInit(): void {
    if (!this.round()) this.loadMyRounds();
  }

  /**
   * Where progress is kept. One store per round: two rounds on one day are two
   * trips with two sets of material, and a shared key would have them ticking
   * off each other's gather list.
   *
   * Nothing at all while only looking. What a planner clicks through is their
   * own browser, not the technician's progress, and writing it would leave the
   * technician's own store overwritten by somebody reading over their shoulder.
   */
  private storageKey(): string | null {
    if (this.readOnly()) return null;
    const round = this.activeRound();
    return round ? `dcim_round_${round.personId}_${round.datacenter}_${round.day}` : null;
  }

  // Resolves the saved ids back onto positions in the freshly loaded list. A
  // task or step that is gone (finished, reassigned) simply falls back to the
  // start rather than resuming at whatever now sits at that index.
  private restoreProgress(tasks: Task[]): void {
    const key = this.storageKey();
    if (!key || tasks.length === 0) return;
    const raw = localStorage.getItem(key);
    if (!raw) return;
    try {
      const saved = JSON.parse(raw) as Partial<ProgressSnapshot>;
      const ti = tasks.findIndex((t) => t.id === saved.taskId);
      const taskIdx = ti >= 0 ? ti : 0;
      const si = tasks[taskIdx].steps.findIndex((s) => s.id === saved.stepId);
      const stepIdx = si >= 0 ? si : 0;
      const keys = new Set(this.gatherItems().map((i) => i.key));

      this.phase.set(saved.phase === 'task' && ti >= 0 ? 'task' : 'gather');
      this.currentTaskIndex.set(taskIdx);
      this.currentStepIndex.set(stepIdx);
      if (saved.phase === 'task' && ti >= 0) this.markReached(taskIdx, stepIdx);
      this.checkedItems.set(new Set((saved.checkedItems ?? []).filter((k) => keys.has(k))));
      this.gatherCompleted.set(!!saved.gatherCompleted);
    } catch {
      // Corrupt/incompatible snapshot — ignore and start fresh.
    }
  }

  private clearProgress(): void {
    const key = this.storageKey();
    if (key) localStorage.removeItem(key);
  }

  /**
   * Who I am, and then my rounds. The walkthrough itself is loaded by the
   * effect that watches which round is active.
   */
  private async loadMyRounds(): Promise<void> {
    try {
      // The auth session carries the identity-provider subject; the task's
      // assignee is a DCIM user id, so resolve one onto the other first.
      const me = await firstValueFrom(this.userApi.getCurrentUser());
      const meId = me.user?.id;
      this.meName.set(me.user ? UserApiService.mapUser(me.user).name : '');
      if (!meId) {
        this.loadError.set(NO_DIRECTORY_ENTRY);
        return;
      }
      this.meId.set(meId);
      this.roundsService.load();
      this.loadError.set(null);
    } catch (err) {
      const message = connectErrorMessage(err);
      // eslint-disable-next-line no-console
      console.error(message);
      // Surfaced rather than swallowed: with nothing on screen and no error the
      // page is indistinguishable from "you have nothing to do".
      //
      // GetCurrentUser answers NotFound for a caller who is authenticated but
      // absent from the DCIM roster, which is an ordinary provisioning state
      // rather than a fault — say so instead of showing the raw RPC message.
      this.loadError.set(
        err instanceof ConnectError && err.code === Code.NotFound ? NO_DIRECTORY_ENTRY : message,
      );
      this.toast.error('Could not load your rounds');
    }
  }

  /**
   * The steps of every task, through a bounded pool. One ListTaskSteps call per
   * task is fine for a technician's open work, but that is a property of
   * today's data rather than a ceiling, so put a real one on it.
   *
   * A partial answer fails the whole load. A walkthrough with silent gaps in it
   * is worse than one that did not load: it would walk somebody straight past
   * work nobody flagged.
   */
  private async withSteps(
    sources: { id: string; title: string; priority: TaskPriorityLabel; tags: string[]; location: string }[],
  ): Promise<Task[]> {
    const completed = new Set<string>();
    const settled = await settledPool(sources, STEP_FETCH_CONCURRENCY, async (t) => {
      const stepsRes = await firstValueFrom(this.taskStepApi.listTaskSteps(t.id));
      const steps: Step[] = stepsRes.steps.map((s, si) => {
        const art = TaskManagementTechnicianComponent.illustrationFor(si);
        if (s.completed) completed.add(s.id);
        return {
          id: s.id,
          title: s.title,
          description: s.description,
          icon: art.icon,
          svg: art.svg,
        };
      });
      return {
        id: t.id,
        title: t.title,
        priority: TaskManagementTechnicianComponent.mapPriority(t.priority),
        tags: t.tags,
        location: t.location,
        steps: steps.length ? steps : [TaskManagementTechnicianComponent.wholeTaskStep(t)],
      };
    });

    const failure = settled.find((r) => r.status === 'rejected');
    if (failure) throw failure.reason;

    this.completedSteps.set(completed);
    return settled.flatMap((r) => (r.status === 'fulfilled' ? [r.value] : []));
  }

  /** A task nobody wrote steps for, as the one step it is. */
  private static wholeTaskStep(task: { id: string; title: string }): Step {
    const art = TaskManagementTechnicianComponent.illustrationFor(0);
    return {
      id: `task:${task.id}`,
      title: task.title,
      description: '',
      icon: art.icon,
      svg: art.svg,
      whole: true,
    };
  }

  /**
   * One round, walked or read. Its tasks come composed and in the order you
   * walk them, so there is nothing to filter or sort here.
   */
  private async loadRound(round: Round): Promise<void> {
    try {
      const tasks = await this.withSteps(
        round.tasks.map((t) => ({
          id: t.id,
          title: t.title,
          priority: t.priority,
          tags: [...t.tags],
          location: t.location,
        })),
      );
      this.tasks.set(tasks);
      // What was ticked off the gather list lives in the technician's own
      // browser, so a round read from elsewhere cannot know it. What it can
      // know is whether the round has started: a step done means somebody went
      // to the floor, and you do not get there without your material.
      this.gatherCompleted.set(this.completedSteps().size > 0);
      this.restoreProgress(tasks);
      // Opened where the work stands rather than at the top: a round is read to
      // see how far it has come, and the gather list is not that. Reading one
      // keeps no progress of its own, so there is no snapshot to open it on.
      // A round nobody has started opens on the gather step, because that is
      // where it starts.
      if (this.gatherCompleted()) {
        const next = tasks.findIndex((_, i) => !this.isTaskDone(i));
        this.openTask(next === -1 ? Math.max(0, tasks.length - 1) : next);
      }
      this.loadError.set(null);
      this.hydrated.set(true);
    } catch (err) {
      const message = connectErrorMessage(err);
      // eslint-disable-next-line no-console
      console.error(message);
      this.loadError.set(message);
    }
  }

  retryLoad(): void {
    this.loadError.set(null);
    this.loadMyRounds();
  }

  /** Creation time in epoch ms; 0 for a task the API sent without one. */
  private static createdMs(t: ProtoTask): number {
    return t.created ? timestampDate(t.created).getTime() : 0;
  }

  private static mapPriority(p: TaskPriorityLabel): 'urgent' | 'high' | 'normal' {
    if (p === 'Urgent') return 'urgent';
    if (p === 'High') return 'high';
    return 'normal';
  }

  private static illustrationFor(index: number): { icon: string; svg: string } {
    const pool = TaskManagementTechnicianComponent.STEP_ILLUSTRATIONS;
    return pool[index % pool.length] ?? { icon: 'info-circle', svg: '' };
  }

  // ── State signals ──
  readonly phase = signal<Phase>('gather');

  readonly currentTaskIndex = signal(0);

  readonly currentStepIndex = signal(0);

  readonly checkedItems = signal(new Set<string>());

  readonly gatherCompleted = signal(false);

  readonly showCompleteScreen = signal(false);

  readonly menuOpen = signal(false);


  readonly noteText = signal('');


  readonly loadError = signal<string | null>(null);

  // Guards the auto-save effect from writing an empty/default snapshot over a
  // saved one before restoreProgress() has had a chance to run.
  private readonly hydrated = signal(false);

  // Ids of the steps completed so far. A signal (rather than a plain Map) so the
  // header progress bar recomputes as steps are ticked off — the app is zoneless,
  // so mutating a non-reactive collection would never repaint it. Keyed by step
  // id so it survives the list being re-ordered or re-fetched.
  private readonly completedSteps = signal(new Set<string>());

  // Set while a step completion is in flight, to keep a double-press from
  // skipping a step.
  readonly savingStep = signal(false);

  // ── Computed ──
  readonly currentTask = computed(() => this.tasks()[this.currentTaskIndex()]);

  /**
   * The work, counted in steps, with the gather step left out of it.
   *
   * Collecting material is preparation for a round rather than part of it, and
   * counting it puts you at 1 of 21 for having picked up a screwdriver. It also
   * keeps this the same number the list of rounds shows, which cannot know
   * whether somebody ticked their gather list: that lives in their browser.
   */
  readonly totalSteps = computed(() => this.tasks().reduce((s, t) => s + t.steps.length, 0));

  readonly completedCount = computed(() => this.completedSteps().size);

  readonly progressPct = computed(() => {
    const total = this.totalSteps();
    return total ? (this.completedCount() / total) * 100 : 0;
  });

  readonly showCompleteBtn = computed(() => {
    const p = this.phase();
    if (p !== 'task') return false;
    const ti = this.currentTaskIndex();
    const si = this.currentStepIndex();
    const tasks = this.tasks();
    if (tasks.length === 0) return false;
    return ti === tasks.length - 1 && si === tasks[ti].steps.length - 1;
  });

  // ── Methods ──
  toggleMenu(event: Event): void {
    event.stopPropagation();
    this.menuOpen.update((v) => !v);
  }

  /** Where a task is, written as the path the tags use: one rack reads the same
   *  wherever you meet it. */
  readonly taskPlace = (task: { tags: string[]; location: string }): string =>
    taskTags(task).find((tag) => tag.includes('/')) ?? task.location;

  /**
   * How far along the row is that carries what a step holds. It has no dot, so
   * its status colors the whole stretch: covered when the next step is already
   * reached, still ahead when this is where the going stops.
   */
  contentStatus(taskIdx: number, stepIdx: number): 'past' | 'current' {
    const front = this.front();
    const behind = front.taskIdx > taskIdx || (front.taskIdx === taskIdx && front.stepIdx > stepIdx);
    return behind ? 'past' : 'current';
  }

  /**
   * Where the row carrying a step's card sits in the track. Under the last step
   * of the last task there is nothing below it, so it draws no line at all: a
   * line running past the end promises a step that is not coming.
   */
  contentPosition(taskIdx: number, stepIdx: number): 'only' | null {
    const tasks = this.tasks();
    const steps = tasks[taskIdx]?.steps.length ?? 0;
    return taskIdx === tasks.length - 1 && stepIdx === steps - 1 ? 'only' : null;
  }

  /** The same for the gather list: covered once there is a task under way. */
  gatherContentStatus(): 'past' | 'current' {
    return this.front().gather ? 'current' : 'past';
  }

  /**
   * How much of the track a step has covered.
   *
   * `both` when the going carries on below the step you are on, which happens
   * when you step back into one you already finished. `top` on the last step
   * that is done with nothing covered below it: the track stops at the dot
   * rather than running half a row past the end of the going. Anything else is
   * what the status says on its own.
   */
  /** Whether you can go to this step: anything up to and including where the
   *  work stands. Skipping one without ticking it off still means you passed
   *  it, so it stays a place you can walk back into. */
  stepReachable(taskIdx: number, stepIdx: number): boolean {
    // Reading it through, everything is open: there is no work to skip past,
    // and Next walks into what is still ahead anyway.
    if (this.readOnly()) return true;
    const front = this.front();
    if (front.taskIdx > taskIdx) return true;
    return front.taskIdx === taskIdx && stepIdx <= front.stepIdx;
  }

  /** The track's own words for where you are. Being here wins over having been
   *  here: you can step back into a finished step, and then it is the one you
   *  are on. */
  /**
   * Where the work stands, which is not the same as what you have open.
   *
   * The track carries the first: everything up to here is behind you, the rest
   * is ahead. Clicking back into step 1 to read it again does not move that —
   * the process is still at step 2, so 2 stays current and 1 reads as past.
   * What you have open is a selection, and that shows itself by the card that
   * hangs under it.
   */
  private readonly reached = signal<{ taskIdx: number; stepIdx: number } | null>(null);

  /**
   * How far the work has come. Pressing on moves it along; clicking back into a
   * step you finished to read it again does not, and neither does a step you
   * land on that happens to be ticked off already — you are still standing
   * there. It only ever moves forward.
   */
  private readonly front = computed(() => {
    const tasks = this.tasks();
    const reached = this.reached();
    if (reached && tasks[reached.taskIdx]) {
      return { gather: false, taskIdx: reached.taskIdx, stepIdx: reached.stepIdx };
    }
    if (!this.gatherCompleted()) return { gather: true, taskIdx: -1, stepIdx: -1 };
    const taskIdx = tasks.findIndex((_, i) => !this.isTaskDone(i));
    if (taskIdx === -1) return { gather: false, taskIdx: -1, stepIdx: -1 };
    const steps = tasks[taskIdx].steps;
    const stepIdx = steps.findIndex((_, i) => !this.isStepDone(taskIdx, i));
    return { gather: false, taskIdx, stepIdx: stepIdx === -1 ? steps.length - 1 : stepIdx };
  });

  /**
   * The notes on the task you have open, read where they were written: one flat
   * stream per task, each line naming the step it came from. Per step would ask
   * for a note that knows its step, and a `Note` carries an entity type that
   * stops at the task.
   */
  readonly taskNotes = signal<NoteComment[]>([]);

  /** When a note was written, as you would say it out loud. */
  readonly noteWhen = (note: NoteComment): string => {
    if (note.daysAgo === 0) return 'today';
    if (note.daysAgo === 1) return 'yesterday';
    return `${note.daysAgo} days ago`;
  };

  private loadNotes(taskId: string): void {
    firstValueFrom(this.noteApi.listNotesForTask(taskId))
      .then((res) => this.taskNotes.set(res.notes.map((n) => NoteApiService.mapNote(n))))
      // A note that will not load is not worth stopping the walkthrough for.
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  private readonly roundsService = inject(RoundsService);

  /** Whose work this is, when it is your own rather than a round you opened. */
  private readonly meName = signal('');

  /** Me, as the tasks know me: the session carries an identity-provider
   *  subject, a task carries a DCIM user id. */
  private readonly meId = signal('');

  /** The round picked from the menu, when nobody handed one in. */
  private readonly chosenRound = signal<Round | null>(null);

  /**
   * The round being walked or read: the one handed in, or the one picked from
   * the menu. A round rather than "everything assigned to me", because a round
   * is one trip to one place on one day, and that is what the gather step is
   * for. Without it the list held two data centers at once and collecting
   * material for it meant nothing.
   */
  readonly activeRound = computed(() => this.round() ?? this.chosenRound());

  /** My rounds, for the menu beside the walkthrough. */
  readonly myRounds = computed(() =>
    this.roundsService.rounds().filter((r) => r.personId === this.meId()),
  );

  /** False until the rounds have been read once, so an empty menu is only
   *  called empty when it is known to be. */
  readonly roundsLoaded = this.roundsService.loaded;

  isActiveRound(round: Round): boolean {
    return this.activeRound()?.key === round.key;
  }

  /** A round in the menu, as its place and its day. */
  readonly roundLabel = (round: Round): string =>
    `${round.datacenter} \u00b7 ${dayLabel(round.day, new Date().toISOString().slice(0, 10))}`;

  roundProgress(round: Round): string {
    const p = this.roundsService.taskProgress(round);
    return `${p.done}/${p.total}`;
  }

  chooseRound(round: Round): void {
    this.chosenRound.set(round);
  }

  /**
   * Who walks this, where, and when. It replaces a hardcoded data center name
   * and an invented work order number, which between them said one location for
   * a list that held two.
   */
  /**
   * The round, as its place and its day. Not the person: reading somebody
   * else's round the menu beside it already says whose, and in your own
   * environment your name is the least informative line on the screen. It
   * stands top right instead, where you also sign out.
   */
  protected readonly roundTitle = computed(() => {
    const round = this.activeRound();
    if (!round) return 'My round';
    const count = this.tasks().length;
    return `${this.roundLabel(round)} (${count})`;
  });

  /** Whose round you are reading, when it is not your own. */
  protected readonly roundOwner = computed(() => {
    const round = this.round();
    return round && round.personId !== this.meId() ? round.personName : '';
  });

  protected readonly userName = computed(() => this.auth.user()?.name ?? this.meName());

  async handleLogout(): Promise<void> {
    await this.auth.logout().catch(() => {});
    await this.router.navigate(['/login']);
  }

  /** How far the round has come, beside its name rather than under it: it is
   *  the same kind of fact as the name, not a footnote to it. */
  /** How far the round has come, counted in tasks: that is the unit the menu
   *  beside it counts in, and what somebody asks about a round. */
  protected readonly roundSubtitle = computed(() => {
    const tasks = this.tasks();
    const done = tasks.filter((_, i) => this.isTaskDone(i)).length;
    const owner = this.roundOwner();
    const progress = tasks.length ? `${done} of ${tasks.length} done` : '';
    return [owner, progress].filter(Boolean).join(' \u00b7 ');
  });

  private readonly host = inject(ElementRef<HTMLElement>);

  private readonly cdr = inject(ChangeDetectorRef);

  /**
   * Room above the row you land on: enough to clear the sheet's own title bar,
   * and to leave a glimpse of the step you just finished.
   */
  private static readonly LANDING_OFFSET = 72;

  /**
   * Moves on to a row and puts it at the top of the view, with the card that
   * opens under it in the room below.
   *
   * Nothing scrolls when you press Done, and that is the trouble: the card
   * closes under one row and opens under the next, so the row you are moving to
   * comes up by the height of the card that stood above it, and the screen
   * fills with what used to be far higher up. It reads as being thrown back to
   * the top. The browser would hold the page for us, but Safari has no scroll
   * anchoring, so the view is placed by hand.
   *
   * Rendered and placed in one go, before anything is painted: no animation and
   * no intermediate frame, so the view does not travel there — it is simply
   * there. Animating it made every step look like a fresh page loading.
   */
  private movingTo(row: string, change: () => void): void {
    change();
    // Render now rather than next frame, so the placing lands in the same paint
    // as the change itself.
    this.cdr.detectChanges();
    const anchor = (this.host.nativeElement as HTMLElement).querySelector<HTMLElement>(
      `[data-row="${row}"]`,
    );
    if (!anchor) return;
    // The scroller belongs to whatever holds this view, and inside a sheet it
    // sits behind a shadow boundary, so scrollIntoView is the only handle on it.
    // scroll-margin is what keeps the row clear of the bar over the scrollport.
    anchor.style.scrollMarginTop = `${TaskManagementTechnicianComponent.LANDING_OFFSET}px`;
    anchor.scrollIntoView({ block: 'start' });
    anchor.style.removeProperty('scroll-margin-top');
  }

  /** Records where you have got to, never back: a step you already finished
   *  does not pull the work backwards when you walk into it again. */
  private markReached(taskIdx: number, stepIdx: number): void {
    const reached = this.reached();
    const ahead = !reached
      || taskIdx > reached.taskIdx
      || (taskIdx === reached.taskIdx && stepIdx > reached.stepIdx);
    if (ahead) this.reached.set({ taskIdx, stepIdx });
  }

  /**
   * Whether the gather step can be opened from where you are. Walking, once you
   * have been past it; reading, always, like every other row: there is nothing
   * to skip when you are not doing the work.
   */
  canOpenGather(): boolean {
    if (this.phase() === 'gather') return false;
    return this.readOnly() || this.gatherCompleted();
  }

  /** The gather step on the track: behind you once you have gone past it. */
  gatherStatus(): 'past' | 'current' {
    return this.front().gather ? 'current' : 'past';
  }

  /**
   * How far the track is covered at a task, measured against the front and
   * nothing else.
   *
   * Being ticked off does not make a row blue. A row you have not reached sits
   * under a stretch of grey line, and a blue dot there would draw an arrival
   * without the journey to it. What is done shows itself with a check mark;
   * the track shows how far the work has come.
   */
  taskStatus(taskIdx: number): 'past' | 'current' | 'future' {
    const front = this.front();
    if (front.taskIdx === taskIdx) return 'current';
    return front.taskIdx > taskIdx ? 'past' : 'future';
  }

  /** The same for a step: behind the front, on it, or still ahead. */
  stepStatus(taskIdx: number, stepIdx: number): 'past' | 'current' | 'future' {
    const front = this.front();
    if (front.taskIdx === taskIdx && front.stepIdx === stepIdx) return 'current';
    const behind = front.taskIdx > taskIdx
      || (front.taskIdx === taskIdx && front.stepIdx > stepIdx);
    return behind ? 'past' : 'future';
  }

  /**
   * Whether the track runs on under a task's dot in its own color.
   *
   * The steps of the task you are in hang below its dot, so that stretch is
   * inside the task and covered. Collapsed there is nothing of its own down
   * there, and a covered line would promise a step that is not on screen.
   */
  taskLine(taskIdx: number): 'both' | null {
    if (this.taskStatus(taskIdx) !== 'current') return null;
    const opensSteps = this.isTaskActive(taskIdx) && this.tasks()[taskIdx].steps.length > 0;
    return opensSteps ? 'both' : null;
  }

  /**
   * Where a row sits in the track: the first has no line above it, the last
   * none below, and a track that ran past its end would promise a step that
   * is not there.
   */
  trackPosition(row: 'task' | 'step', taskIdx: number, stepIdx = -1): 'first' | 'between' | 'last' {
    const tasks = this.tasks();
    const lastTask = tasks.length - 1;
    if (row === 'task') {
      const opensSteps = this.isTaskActive(taskIdx) && tasks[taskIdx].steps.length > 0;
      return taskIdx === lastTask && !opensSteps ? 'last' : 'between';
    }
    const steps = tasks[taskIdx]?.steps.length ?? 0;
    return taskIdx === lastTask && stepIdx === steps - 1 ? 'last' : 'between';
  }

  isTaskActive(taskIdx: number): boolean {
    return this.phase() === 'task' && this.currentTaskIndex() === taskIdx;
  }

  isTaskDone(taskIdx: number): boolean {
    const task = this.tasks()[taskIdx];
    if (!task || task.steps.length === 0) return false;
    const done = this.completedSteps();
    return task.steps.every((s) => done.has(s.id));
  }

  isStepActive(taskIdx: number, stepIdx: number): boolean {
    return this.isTaskActive(taskIdx) && this.currentStepIndex() === stepIdx;
  }

  isStepDone(taskIdx: number, stepIdx: number): boolean {
    const step = this.tasks()[taskIdx]?.steps[stepIdx];
    return step ? this.completedSteps().has(step.id) : false;
  }

  jumpToStep(taskIdx: number, stepIdx: number): void {
    this.phase.set('task');
    this.currentTaskIndex.set(taskIdx);
    this.currentStepIndex.set(stepIdx);
  }

  safeSvg(svg: string, isDark = false): SafeHtml {
    const source = isDark
      ? svg.replace(/(fill|stroke)="([^"]+)"/g, (match, _attr, color) => {
          const dark = TaskManagementTechnicianComponent.SVG_DARK_MAP[color.toLowerCase()];
          return dark ? match.replace(`"${color}"`, `"${dark}"`) : match;
        })
      : svg;
    return this.sanitizer.bypassSecurityTrustHtml(source);
  }

  toggleGatherItem(key: string): void {
    this.checkedItems.update((set) => {
      const next = new Set(set);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }

  onGatherCheckbox(key: string, checked: boolean): void {
    this.checkedItems.update((set) => {
      const next = new Set(set);
      if (checked) next.add(key);
      else next.delete(key);
      return next;
    });
  }

  /** Whether you have been in this task at all: one step ticked off is enough
   *  to make it a place you can go back to. */
  /** The same for a task: everything up to and including where the work stands. */
  taskStarted(taskIdx: number): boolean {
    if (this.readOnly()) return true;
    return this.front().taskIdx >= taskIdx;
  }

  /**
   * Back into a task you left halfway, at the step you left it on: the first
   * one that is not done yet, or the last step when the whole task is. Landing
   * on the first step again would make you walk past everything you already
   * did.
   */
  openTask(taskIdx: number): void {
    const steps = this.tasks()[taskIdx]?.steps ?? [];
    const next = steps.findIndex((step) => !this.completedSteps().has(step.id));
    const stepIdx = next === -1 ? Math.max(0, steps.length - 1) : next;
    this.jumpToStep(taskIdx, stepIdx);
    // Reading a round does not move it along. Walking into a task is what puts
    // the work there; opening one to look at it is not, and every row is open
    // to a reader, including the ones still ahead.
    if (!this.readOnly()) this.markReached(taskIdx, stepIdx);
  }

  /** Back to gathering, the way Previous gets there from the first step: the
   *  list you ticked off is still worth a second look. */
  goToGather(): void {
    this.phase.set('gather');
  }

  /** Whether there is anything after the step you are looking at. */
  readonly atEnd = computed(() => {
    const tasks = this.tasks();
    if (this.phase() === 'gather') return tasks.length === 0;
    const ti = this.currentTaskIndex();
    const si = this.currentStepIndex();
    return ti >= tasks.length - 1 && si >= (tasks[ti]?.steps.length ?? 1) - 1;
  });

  /**
   * The mirror of Previous, for reading a round through. It only moves what you
   * are looking at: where the work stands is the technician's, and a planner
   * paging through it does not push it along.
   */
  pressNext(): void {
    if (this.atEnd()) return;
    if (this.phase() === 'gather') {
      this.jumpToStep(0, 0);
      return;
    }
    const ti = this.currentTaskIndex();
    const si = this.currentStepIndex();
    if (si < this.tasks()[ti].steps.length - 1) this.currentStepIndex.update((v) => v + 1);
    else this.jumpToStep(ti + 1, 0);
  }

  async pressDone(): Promise<void> {
    if (this.phase() === 'gather') {
      if (this.tasks().length === 0) {
        // Covers both "nothing assigned" and "everything assigned has no steps
        // to walk through", which read the same from here.
        this.toast.info('No tasks to walk through right now');
        return;
      }
      this.gatherCompleted.set(true);
      // Where the work stands, not the very beginning: you can come back to the
      // gather list halfway through and pressing on should carry on from there.
      const tasks = this.tasks();
      const next = tasks.findIndex((_, i) => !this.isTaskDone(i));
      const target = next === -1 ? Math.max(0, tasks.length - 1) : next;
      this.movingTo(`task-${target}`, () => this.openTask(target));
      return;
    }

    if (this.savingStep()) return;

    const ti = this.currentTaskIndex();
    const si = this.currentStepIndex();
    const task = this.tasks()[ti];
    const step = task?.steps[si];
    if (!task || !step) return;

    const lastStep = si === task.steps.length - 1;

    this.savingStep.set(true);
    try {
      // Stay put on a failed write — advancing (or showing the completion
      // screen) would lose the technician's work with nothing to show for it.
      if (!(await this.persistStepDone(step))) return;
      if (lastStep) await this.closeTask(task);
    } finally {
      this.savingStep.set(false);
    }

    if (!lastStep) {
      this.movingTo(`step-${ti}-${si + 1}`, () => {
        this.currentStepIndex.update((v) => v + 1);
        this.markReached(ti, si + 1);
      });
    } else if (ti < this.tasks().length - 1) {
      // The same landing as clicking the task: its first step that still needs
      // doing. Step 0 regardless would walk you into work already recorded as
      // done, and leave that step blue below the one you stand on.
      this.movingTo(`task-${ti + 1}`, () => this.openTask(ti + 1));
    } else {
      this.showCompleteScreen.set(true);
      this.clearProgress();
    }
  }

  /**
   * Ticks the step off optimistically and persists it. Returns false — having
   * rolled the tick back — when the write failed, so the caller can stay on the
   * step instead of advancing past work that was never recorded.
   */
  private async persistStepDone(step: Step): Promise<boolean> {
    this.completedSteps.update((set) => new Set(set).add(step.id));
    // A task that stands for itself has no step record behind it; closing the
    // task is what records it, and that happens a line later in pressDone.
    if (step.whole) return true;
    try {
      await firstValueFrom(this.taskStepApi.updateTaskStep(step.id, true));
      return true;
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error(connectErrorMessage(err));
      this.completedSteps.update((set) => {
        const next = new Set(set);
        next.delete(step.id);
        return next;
      });
      this.toast.error('Could not save this step — try again');
      return false;
    }
  }

  /**
   * Closes the task once its last step is done — otherwise a fully walked task
   * sits in In Progress forever, and reappears in this same walkthrough on the
   * next login. Only the status is sent, so nothing else on the task is
   * overwritten with the technician's stale view of it.
   *
   * A failure here is reported but never rolls the step back: the step really is
   * done, and re-opening it would be a lie the technician cannot act on.
   */
  private async closeTask(task: Task): Promise<void> {
    try {
      await firstValueFrom(this.taskApi.updateTask(task.id, { status: 'Done' }));
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error(connectErrorMessage(err));
      this.toast.warning('Step saved, but the task could not be closed');
    }
  }

  saveNote(): void {
    const text = this.noteText().trim();
    if (!text) return;
    const task = this.currentTask();
    if (!task) return;
    // One flat stream per task: a note hangs off the task, is written from the
    // task, and is read there. Nothing names a step, so nothing has to be
    // parsed back out of the text later.
    firstValueFrom(this.noteApi.createNoteForTask(task.id, text))
      .then(() => {
        this.noteText.set('');
        this.loadNotes(task.id);
      })
      .catch((err) => {
        // eslint-disable-next-line no-console
        console.error(connectErrorMessage(err));
        this.toast.error('Could not save note');
      });
  }
}
