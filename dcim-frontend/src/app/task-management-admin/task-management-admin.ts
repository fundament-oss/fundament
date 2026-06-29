import {
  Component,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
  OnInit,
  signal,
  computed,
  inject,
  viewChild,
  ElementRef,
} from '@angular/core';
import { firstValueFrom, Observable } from 'rxjs';
import { timestampDate } from '@bufbuild/protobuf/wkt';
import type { Note as ProtoNote } from '../../generated/v1/note_pb';
import DropdownSyncDirective from '../shared/dropdown-sync.directive';
import AuthService from '../auth.service';
import TaskApiService, {
  TaskData,
  TaskInput,
  TaskCategoryLabel,
  TaskPriorityLabel,
  TaskStatusLabel,
} from '../task-management/task-api.service';
import UserApiService, { RosterUser } from '../task-management/user-api.service';
import NoteApiService from '../inventory/note-api.service';
import connectErrorMessage from '../../connect/error';

type Technician = RosterUser;

interface Note {
  author: string;
  text: string;
  time: string;
}

interface Task extends TaskData {
  notes: Note[];
  notesLoaded: boolean;
}

interface StatusStyle {
  bg: string;
  text: string;
  dot: string;
  kanbanAccent: string;
  kanbanBorder: string;
}

interface PriorityStyle {
  bg: string;
  text: string;
  dot: string;
  ring: string;
}

interface NlddSheet extends HTMLElement {
  show(): void;
  hide(): void;
}

@Component({
  selector: 'app-task-management-admin',
  templateUrl: './task-management-admin.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [DropdownSyncDirective],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: {
    class: 'flex flex-col bg-white dark:bg-gray-950 text-slate-900 dark:text-white',
    '(document:keydown.escape)': 'onEscape()',
  },
})
export default class TaskManagementAdminComponent implements OnInit {
  private readonly taskApi = inject(TaskApiService);

  private readonly userApi = inject(UserApiService);

  private readonly noteApi = inject(NoteApiService);

  private readonly auth = inject(AuthService);

  readonly technicians = signal<Technician[]>([]);

  tasks = signal<Task[]>([]);

  ngOnInit(): void {
    this.loadUsers();
    this.loadTasks();
  }

  private loadUsers(): void {
    firstValueFrom(this.userApi.listUsers())
      .then((res) => this.technicians.set(res.users.map((u) => UserApiService.mapUser(u))))
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  private loadTasks(): void {
    firstValueFrom(this.taskApi.listTasks())
      .then((res) =>
        this.tasks.set(
          res.tasks.map((t) => ({ ...TaskApiService.mapTask(t), notes: [], notesLoaded: false })),
        ),
      )
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  readonly statusStyles: Record<string, StatusStyle> = {
    Ready: {
      bg: 'bg-slate-100 dark:bg-gray-800',
      text: 'text-slate-600 dark:text-gray-300',
      dot: 'bg-slate-400',
      kanbanAccent: 'bg-slate-400',
      kanbanBorder: 'border-slate-200 dark:border-gray-800',
    },
    'In Progress': {
      bg: 'bg-indigo-50 dark:bg-indigo-950',
      text: 'text-indigo-700 dark:text-indigo-300',
      dot: 'bg-indigo-500',
      kanbanAccent: 'bg-indigo-500',
      kanbanBorder: 'border-indigo-200 dark:border-indigo-800',
    },
    Review: {
      bg: 'bg-amber-50 dark:bg-amber-950',
      text: 'text-amber-700 dark:text-amber-300',
      dot: 'bg-amber-500',
      kanbanAccent: 'bg-amber-500',
      kanbanBorder: 'border-amber-200 dark:border-amber-800',
    },
    Blocked: {
      bg: 'bg-red-50 dark:bg-red-950',
      text: 'text-red-700 dark:text-red-300',
      dot: 'bg-red-500',
      kanbanAccent: 'bg-red-500',
      kanbanBorder: 'border-red-200 dark:border-red-800',
    },
    Done: {
      bg: 'bg-emerald-50 dark:bg-emerald-950',
      text: 'text-emerald-700 dark:text-emerald-300',
      dot: 'bg-emerald-500',
      kanbanAccent: 'bg-emerald-500',
      kanbanBorder: 'border-emerald-200 dark:border-emerald-800',
    },
  };

  readonly priorityStyles: Record<string, PriorityStyle> = {
    Critical: {
      bg: 'bg-red-50 dark:bg-red-950',
      text: 'text-red-700 dark:text-red-300',
      dot: 'bg-red-500',
      ring: 'ring-red-200/80 dark:ring-red-800/80',
    },
    High: {
      bg: 'bg-orange-50 dark:bg-orange-950',
      text: 'text-orange-700 dark:text-orange-300',
      dot: 'bg-orange-500',
      ring: 'ring-orange-200/80 dark:ring-orange-800/80',
    },
    Medium: {
      bg: 'bg-yellow-50 dark:bg-yellow-950',
      text: 'text-yellow-700 dark:text-yellow-300',
      dot: 'bg-yellow-400',
      ring: 'ring-yellow-200/80 dark:ring-yellow-800/80',
    },
    Low: {
      bg: 'bg-slate-100 dark:bg-gray-800',
      text: 'text-slate-500 dark:text-gray-400',
      dot: 'bg-slate-400',
      ring: 'ring-slate-200/80 dark:ring-gray-700/80',
    },
  };

  readonly categoryIcons: Record<string, string> = {
    Hardware: 'cylinder-split',
    Network: 'list',
    Cooling: 'cloud',
    Power: 'lock-closed',
    Security: 'shield-check-mark',
    Other: 'ellipsis',
  };

  readonly kanbanColumns: TaskStatusLabel[] = ['Ready', 'In Progress', 'Review', 'Blocked', 'Done'];

  readonly priorities: TaskPriorityLabel[] = ['Critical', 'High', 'Medium', 'Low'];

  readonly taskCategories: TaskCategoryLabel[] = [
    'Hardware',
    'Network',
    'Cooling',
    'Power',
    'Security',
    'Other',
  ];

  private readonly dateLocale = 'en-US';

  currentView = signal<'list' | 'kanban'>('list');

  searchQuery = signal('');

  statusFilter = signal('all');

  priorityFilter = signal('all');

  categoryFilter = signal('all');

  selectedTasks = signal<Set<string>>(new Set());

  detailTaskId = signal<string | null>(null);

  editingTaskId = signal<string | null | undefined>(undefined);

  editFormTitle = signal('');

  editFormDescription = signal('');

  editFormStatus = signal<TaskStatusLabel>('Ready');

  editFormPriority = signal<TaskPriorityLabel>('Medium');

  editFormCategory = signal<TaskCategoryLabel>('Hardware');

  editFormDue = signal('');

  editFormLocation = signal('');

  editFormAssignee = signal<string | null>(null);

  editTitleTouched = signal(false);

  editTitleInvalid = computed(() => this.editTitleTouched() && !this.editFormTitle().trim());

  newNoteText = signal('');

  toastMessage = signal<string | null>(null);

  private toastTimeout: number | undefined;

  filteredTasks = computed(() => {
    const q = this.searchQuery().toLowerCase().trim();
    const st = this.statusFilter();
    const pr = this.priorityFilter();
    const cat = this.categoryFilter();
    return this.tasks().filter((t) => {
      if (st !== 'all' && t.status !== st) return false;
      if (pr !== 'all' && t.priority !== pr) return false;
      if (cat !== 'all' && t.category !== cat) return false;
      if (q && !t.title.toLowerCase().includes(q) && !t.description.toLowerCase().includes(q))
        return false;
      return true;
    });
  });

  statusCounts = computed(() =>
    this.tasks().reduce<Record<string, number>>(
      (acc, t) => ({ ...acc, [t.status]: (acc[t.status] ?? 0) + 1 }),
      {},
    ),
  );

  priorityCounts = computed(() =>
    this.tasks().reduce<Record<string, number>>(
      (acc, t) => ({ ...acc, [t.priority]: (acc[t.priority] ?? 0) + 1 }),
      {},
    ),
  );

  categoryCounts = computed(() =>
    this.tasks().reduce<Record<string, number>>(
      (acc, t) => ({ ...acc, [t.category]: (acc[t.category] ?? 0) + 1 }),
      {},
    ),
  );

  detailTask = computed(() => {
    const id = this.detailTaskId();
    if (id === null) return null;
    return this.tasks().find((t) => t.id === id) ?? null;
  });

  editModalTitle = computed(() => (this.editingTaskId() ? 'Edit task' : 'New task'));

  readonly detailSheetEl = viewChild<ElementRef<NlddSheet>>('detailSheetEl');

  readonly editModalEl = viewChild<ElementRef<NlddSheet>>('editModalEl');

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

  readonly taskDisplayId = (task: Task): string => `T-${task.id.slice(0, 8).toUpperCase()}`;

  isSelected(id: string): boolean {
    return this.selectedTasks().has(id);
  }

  statusStyle(status: string): StatusStyle {
    return this.statusStyles[status] ?? this.statusStyles['Ready'];
  }

  priorityStyle(priority: string): PriorityStyle {
    return this.priorityStyles[priority] ?? this.priorityStyles['Medium'];
  }

  categoryIcon(category: string): string {
    return this.categoryIcons[category] ?? 'ellipsis';
  }

  tasksForColumn(col: string): Task[] {
    return this.filteredTasks().filter((t) => t.status === col);
  }

  setView(view: 'list' | 'kanban'): void {
    this.currentView.set(view);
  }

  toggleSelection(id: string, checked: boolean): void {
    this.selectedTasks.update((set) => {
      const next = new Set(set);
      if (checked) next.add(id);
      else next.delete(id);
      return next;
    });
  }

  toggleSelectAll(checked: boolean): void {
    if (checked) {
      this.selectedTasks.set(new Set(this.tasks().map((t) => t.id)));
    } else {
      this.selectedTasks.set(new Set());
    }
  }

  openDetail(id: string): void {
    this.detailTaskId.set(id);
    this.loadNotes(id);
    this.detailSheetEl()?.nativeElement.show();
  }

  private loadNotes(id: string): void {
    firstValueFrom(this.noteApi.listNotesForTask(id))
      .then((res) => {
        const notes = res.notes.map((n) => TaskManagementAdminComponent.mapNote(n));
        this.tasks.update((tasks) =>
          tasks.map((t) => (t.id === id ? { ...t, notes, notesLoaded: true } : t)),
        );
      })
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  private static mapNote(n: ProtoNote): Note {
    return {
      author: n.createdBy,
      text: n.body,
      time: TaskManagementAdminComponent.relativeTime(
        n.created ? timestampDate(n.created) : new Date(),
      ),
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

  openEditModal(taskId: string | null): void {
    this.editingTaskId.set(taskId);
    const task = taskId !== null ? this.tasks().find((t) => t.id === taskId) : null;
    this.editFormTitle.set(task?.title ?? '');
    this.editFormDescription.set(task?.description ?? '');
    this.editFormStatus.set(task?.status ?? 'Ready');
    this.editFormPriority.set(task?.priority ?? 'Medium');
    this.editFormCategory.set(task?.category ?? 'Hardware');
    this.editFormDue.set(task?.due ?? '');
    this.editFormLocation.set(task?.location ?? '');
    this.editFormAssignee.set(task?.assignee ?? null);
    this.editTitleTouched.set(false);
    this.editModalEl()?.nativeElement.show();
  }

  closeEditModal(): void {
    this.editModalEl()?.nativeElement.hide();
    this.editingTaskId.set(undefined);
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
      category: this.editFormCategory(),
      due: this.editFormDue(),
      location: this.editFormLocation().trim(),
      assignee: this.editFormAssignee(),
    };

    const editingId = this.editingTaskId();
    const request: Observable<unknown> =
      editingId !== null && editingId !== undefined
        ? this.taskApi.updateTask(editingId, input)
        : this.taskApi.createTask(input);

    firstValueFrom(request)
      .then(() => {
        this.loadTasks();
        this.showToast(editingId ? 'Task updated' : 'Task created');
        this.editModalEl()?.nativeElement.hide();
        this.editingTaskId.set(undefined);
      })
      .catch((err) => {
        // eslint-disable-next-line no-console
        console.error(connectErrorMessage(err));
        this.showToast('Could not save task');
      });
  }

  addNote(): void {
    const text = this.newNoteText().trim();
    if (!text) return;
    const id = this.detailTaskId();
    if (id === null) return;
    const author = this.auth.user()?.name ?? 'Admin';
    firstValueFrom(this.noteApi.createNoteForTask(id, text, author))
      .then(() => {
        this.newNoteText.set('');
        this.loadNotes(id);
        this.showToast('Note added');
      })
      .catch((err) => {
        // eslint-disable-next-line no-console
        console.error(connectErrorMessage(err));
        this.showToast('Could not add note');
      });
  }

  showToast(msg: string): void {
    this.toastMessage.set(msg);
    clearTimeout(this.toastTimeout);
    this.toastTimeout = window.setTimeout(() => {
      this.toastMessage.set(null);
    }, 2000);
  }

  noteAuthor(note: Note): { name: string; tech: Technician | null } {
    const tech = this.technicians().find((t) => t.name === note.author) ?? null;
    return { name: note.author || 'Admin', tech };
  }

  onEscape(): void {
    this.detailSheetEl()?.nativeElement.hide();
    this.editModalEl()?.nativeElement.hide();
  }

  statusBadgeClass(status: string): string {
    const s = this.statusStyle(status);
    return `inline-flex items-center gap-1.5 rounded-full ${s.bg} px-2.5 py-0.5 text-xs font-medium ${s.text}`;
  }

  statusDotClass(status: string): string {
    return `h-1.5 w-1.5 rounded-full ${this.statusStyle(status).dot} shrink-0`;
  }

  priorityTextClass(priority: string): string {
    return `inline-flex items-center gap-1.5 text-xs font-medium ${this.priorityStyle(priority).text}`;
  }

  priorityDotClass(priority: string): string {
    return `h-2 w-2 rounded-full ${this.priorityStyle(priority).dot} shrink-0`;
  }

  priorityBadgeClass(priority: string): string {
    const p = this.priorityStyle(priority);
    return `inline-flex items-center gap-1.5 rounded-full ${p.bg} px-2.5 py-0.5 text-xs font-medium ${p.text} ring-1 ${p.ring}`;
  }

  kanbanCardClass(status: string): string {
    const s = this.statusStyle(status);
    return `cursor-pointer rounded-xl border ${s.kanbanBorder} bg-white dark:bg-gray-950 p-3.5 hover:shadow-md hover:shadow-slate-200/80 transition-shadow`;
  }

  detailStatusClass(status: string): string {
    const s = this.statusStyle(status);
    return `inline-flex items-center gap-1.5 rounded-full ${s.bg} px-2.5 py-0.5 text-xs font-medium ${s.text}`;
  }

  detailPriorityClass(priority: string): string {
    const p = this.priorityStyle(priority);
    return `inline-flex items-center gap-1.5 rounded-full ${p.bg} px-2.5 py-0.5 text-xs font-medium ${p.text} ring-1 ${p.ring}`;
  }

  readonly techInitialsClass = (tech: Technician, size = 'h-7 w-7 text-xs'): string =>
    `inline-flex ${size} items-center justify-center rounded-full ${tech.color} text-white font-semibold shrink-0`;

  readonly unassignedAvatarClass = (size = 'h-7 w-7 text-xs'): string =>
    `inline-flex ${size} items-center justify-center rounded-full bg-slate-200 dark:bg-gray-800 text-slate-500 dark:text-gray-400 font-medium shrink-0`;

  shortDate(str: string | null): string {
    if (!str) return '';
    return new Date(`${str}T00:00:00`).toLocaleDateString(this.dateLocale, {
      month: 'short',
      day: 'numeric',
    });
  }
}
