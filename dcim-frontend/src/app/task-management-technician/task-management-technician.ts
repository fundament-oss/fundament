import {
  Component,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
  OnInit,
  signal,
  computed,
  effect,
  inject,
} from '@angular/core';
import { RouterLink } from '@angular/router';
import { DomSanitizer, SafeHtml } from '@angular/platform-browser';
import { Code, ConnectError } from '@connectrpc/connect';
import { firstValueFrom } from 'rxjs';

import ThemeToggleComponent from '../shared/theme-toggle';
import ThemeService from '../theme.service';
import AuthService from '../auth.service';
import TaskApiService, {
  TaskPriorityLabel,
  TaskStatusLabel,
} from '../task-management/task-api.service';
import TaskStepApiService from '../task-management/task-step-api.service';
import UserApiService from '../task-management/user-api.service';
import NoteApiService from '../inventory/note-api.service';
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
}

interface Task {
  id: string;
  title: string;
  priority: 'critical' | 'high' | 'normal';
  category: string;
  location: string;
  steps: Step[];
}

type Phase = 'gather' | 'task';

/** What the technician walks through: the statuses that still need work done. */
const WALKTHROUGH_STATUSES: TaskStatusLabel[] = ['Ready', 'In Progress'];

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
  imports: [RouterLink, ThemeToggleComponent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: {
    class:
      'block bg-neutral-50 dark:bg-gray-900 font-sans text-neutral-900 dark:text-white antialiased',
  },
})
export default class TaskManagementTechnicianComponent implements OnInit {
  private sanitizer = inject(DomSanitizer);

  protected readonly theme = inject(ThemeService);

  private readonly auth = inject(AuthService);

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
      svg: `<svg viewBox="0 0 120 100" fill="none" xmlns="http://www.w3.org/2000/svg" class="h-40 w-full" aria-hidden="true">
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
      svg: `<svg viewBox="0 0 120 100" fill="none" xmlns="http://www.w3.org/2000/svg" class="h-40 w-full" aria-hidden="true">
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
      svg: `<svg viewBox="0 0 120 100" fill="none" xmlns="http://www.w3.org/2000/svg" class="h-40 w-full" aria-hidden="true">
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
      svg: `<svg viewBox="0 0 120 100" fill="none" xmlns="http://www.w3.org/2000/svg" class="h-40 w-full" aria-hidden="true">
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
      svg: `<svg viewBox="0 0 120 100" fill="none" xmlns="http://www.w3.org/2000/svg" class="h-40 w-full" aria-hidden="true">
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
      svg: `<svg viewBox="0 0 120 100" fill="none" xmlns="http://www.w3.org/2000/svg" class="h-40 w-full" aria-hidden="true">
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
      svg: `<svg viewBox="0 0 120 100" fill="none" xmlns="http://www.w3.org/2000/svg" class="h-40 w-full" aria-hidden="true">
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
      svg: `<svg viewBox="0 0 120 100" fill="none" xmlns="http://www.w3.org/2000/svg" class="h-40 w-full" aria-hidden="true">
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

  readonly dcName = 'DC Amsterdam-West';

  // Generic part label per task category, used to build the gather checklist.
  private static readonly CATEGORY_PARTS: Record<string, string> = {
    Hardware: 'Replacement hardware components',
    Network: 'Replacement network device',
    Cooling: 'Cooling spares & filters',
    Power: 'Power components & fuses',
    Security: 'Mounting hardware & cabling',
    Other: 'Task-specific parts',
  };

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
      label: TaskManagementTechnicianComponent.CATEGORY_PARTS[t.category] ?? 'Task-specific parts',
      taskFor: `${t.title} — ${TaskManagementTechnicianComponent.lastLocationSegment(t.location)}`,
    }));
    return [...tools, ...parts];
  });

  private static lastLocationSegment(location: string): string {
    const seg = location.split('·')[1]?.trim();
    return seg ?? location;
  }

  constructor() {
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
    this.loadTasks();
  }

  private storageKey(): string | null {
    const id = this.auth.user()?.id;
    return id ? `dcim_tech_progress_${id}` : null;
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

  private async loadTasks(): Promise<void> {
    try {
      // The auth session carries the identity-provider subject; the task's
      // assignee is a DCIM user id, so resolve one onto the other first.
      const me = await firstValueFrom(this.userApi.getCurrentUser());
      const meId = me.user?.id;
      if (!meId) {
        this.loadError.set(NO_DIRECTORY_ENTRY);
        return;
      }

      const res = await firstValueFrom(this.taskApi.listTasks(meId));
      // Only tasks that still need doing. Without this, tasks the technician
      // already finished (or that are blocked/awaiting review) pad the gather
      // checklist and the linear flow walks them back through completed work.
      const open = res.tasks.filter((t) =>
        WALKTHROUGH_STATUSES.includes(TaskApiService.fromProtoStatus(t.status)),
      );

      // One ListTaskSteps call per task, run in parallel. Bounded in practice by
      // WALKTHROUGH_STATUSES — a technician's open tasks, not the whole board.
      const completed = new Set<string>();
      const tasks = await Promise.all(
        open.map(async (t) => {
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
            priority: TaskManagementTechnicianComponent.mapPriority(
              TaskApiService.fromProtoPriority(t.priority),
            ),
            category: TaskApiService.fromProtoCategory(t.category),
            location: t.location,
            steps,
          };
        }),
      );

      // A task with no steps is dropped rather than walked into: the flow
      // advances one step at a time, so there is nothing for "Done" to complete
      // on such a task and the technician would be stranded on it with no way
      // forward. Tasks are seeded without steps often enough for this to bite.
      const walkable = tasks.filter((t) => t.steps.length > 0);

      this.completedSteps.set(completed);
      this.tasks.set(walkable);
      this.restoreProgress(walkable);
      this.loadError.set(null);
      this.hydrated.set(true);
    } catch (err) {
      const message = connectErrorMessage(err);
      // eslint-disable-next-line no-console
      console.error(message);
      // Surfaced rather than swallowed: with no tasks and no error the page is
      // indistinguishable from "you have nothing to do". hydrated stays false so
      // the auto-save effect cannot overwrite the saved snapshot with an empty one.
      //
      // GetCurrentUser answers NotFound for a caller who is authenticated but
      // absent from the DCIM roster, which is an ordinary provisioning state
      // rather than a fault — say so instead of showing the raw RPC message.
      this.loadError.set(
        err instanceof ConnectError && err.code === Code.NotFound ? NO_DIRECTORY_ENTRY : message,
      );
      this.toast.show('Could not load your tasks');
    }
  }

  retryLoad(): void {
    this.loadError.set(null);
    this.loadTasks();
  }

  private static mapPriority(p: TaskPriorityLabel): 'critical' | 'high' | 'normal' {
    if (p === 'Critical') return 'critical';
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

  readonly showPhotoModal = signal(false);

  readonly showNoteModal = signal(false);

  readonly noteText = signal('');

  readonly photoPreviewUrl = signal<string | null>(null);

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

  readonly totalSteps = computed(() => 1 + this.tasks().reduce((s, t) => s + t.steps.length, 0));

  readonly completedCount = computed(
    () => (this.gatherCompleted() ? 1 : 0) + this.completedSteps().size,
  );

  readonly progressPct = computed(() => (this.completedCount() / this.totalSteps()) * 100);

  readonly showCompleteBtn = computed(() => {
    const p = this.phase();
    if (p !== 'task') return false;
    const ti = this.currentTaskIndex();
    const si = this.currentStepIndex();
    const tasks = this.tasks();
    if (tasks.length === 0) return false;
    return ti === tasks.length - 1 && si === tasks[ti].steps.length - 1;
  });

  readonly currentStepLabel = computed(() => {
    if (this.phase() === 'gather') return 'Gather tools & parts';
    const task = this.tasks()[this.currentTaskIndex()];
    if (!task) return '';
    const si = this.currentStepIndex();
    return `${task.title} — Step ${si + 1}: ${task.steps[si]?.title ?? ''}`;
  });

  // ── Methods ──
  toggleMenu(event: Event): void {
    event.stopPropagation();
    this.menuOpen.update((v) => !v);
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

  pressPrev(): void {
    if (this.phase() === 'gather') return;
    if (this.currentStepIndex() > 0) {
      this.currentStepIndex.update((v) => v - 1);
    } else if (this.currentTaskIndex() > 0) {
      this.currentTaskIndex.update((v) => v - 1);
      this.currentStepIndex.set(this.tasks()[this.currentTaskIndex()].steps.length - 1);
    } else {
      this.phase.set('gather');
    }
  }

  async pressDone(): Promise<void> {
    if (this.phase() === 'gather') {
      if (this.tasks().length === 0) {
        // Covers both "nothing assigned" and "everything assigned has no steps
        // to walk through", which read the same from here.
        this.toast.show('No tasks to walk through right now');
        return;
      }
      if (this.checkedItems().size < this.gatherItems().length) {
        this.toast.show(
          `${this.checkedItems().size}/${this.gatherItems().length} items checked — proceeding`,
        );
      }
      this.gatherCompleted.set(true);
      this.phase.set('task');
      this.currentTaskIndex.set(0);
      this.currentStepIndex.set(0);
      return;
    }

    if (this.savingStep()) return;

    const ti = this.currentTaskIndex();
    const si = this.currentStepIndex();
    const task = this.tasks()[ti];
    const step = task?.steps[si];
    if (!task || !step) return;

    // Mark it done optimistically, but roll back and stay on the step if the
    // write fails — advancing (or showing the completion screen) on a failed
    // call would lose the technician's work with nothing to show for it.
    this.savingStep.set(true);
    this.completedSteps.update((set) => new Set(set).add(step.id));
    try {
      await firstValueFrom(this.taskStepApi.updateTaskStep(step.id, true));
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error(connectErrorMessage(err));
      this.completedSteps.update((set) => {
        const next = new Set(set);
        next.delete(step.id);
        return next;
      });
      this.toast.show('Could not save this step — try again');
      return;
    } finally {
      this.savingStep.set(false);
    }

    if (si < task.steps.length - 1) {
      this.currentStepIndex.update((v) => v + 1);
    } else if (ti < this.tasks().length - 1) {
      this.currentTaskIndex.update((v) => v + 1);
      this.currentStepIndex.set(0);
    } else {
      this.showCompleteScreen.set(true);
      this.clearProgress();
    }
  }

  openPhotoModal(): void {
    this.photoPreviewUrl.set(null);
    this.showPhotoModal.set(true);
  }

  closePhotoModal(): void {
    this.showPhotoModal.set(false);
  }

  // There is no photo upload endpoint yet, so the capture below is preview-only
  // and the modal says so. Deliberately no "Save": confirming a write that never
  // happens would cost the technician the evidence they think they filed.
  onPhotoSelected(event: Event): void {
    const file = (event.target as HTMLInputElement).files?.[0];
    if (file) {
      const reader = new FileReader();
      reader.onload = (ev) => {
        this.photoPreviewUrl.set(ev.target!.result as string);
      };
      reader.readAsDataURL(file);
    }
  }

  openNoteModal(): void {
    this.showNoteModal.set(true);
  }

  closeNoteModal(): void {
    this.showNoteModal.set(false);
    this.noteText.set('');
  }

  saveNote(): void {
    const text = this.noteText().trim();
    if (!text) return;
    const task = this.currentTask();
    if (!task) return;
    firstValueFrom(this.noteApi.createNoteForTask(task.id, text))
      .then(() => {
        this.noteText.set('');
        this.showNoteModal.set(false);
        this.toast.show('Note saved');
      })
      .catch((err) => {
        // eslint-disable-next-line no-console
        console.error(connectErrorMessage(err));
        this.toast.show('Could not save note');
      });
  }

  onModalBackdropClick(event: Event, modal: 'photo' | 'note'): void {
    if (event.target === event.currentTarget) {
      if (modal === 'photo') this.showPhotoModal.set(false);
      else {
        this.showNoteModal.set(false);
        this.noteText.set('');
      }
    }
  }
}
