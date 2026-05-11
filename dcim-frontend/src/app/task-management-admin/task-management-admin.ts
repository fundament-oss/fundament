import {
  Component,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
  signal,
  computed,
  viewChild,
  ElementRef,
} from '@angular/core';

interface Technician {
  id: number;
  name: string;
  initials: string;
  color: string;
  available: boolean;
}

interface Note {
  author: number | null;
  text: string;
  time: string;
}

interface Task {
  id: number;
  title: string;
  description: string;
  status: string;
  priority: string;
  category: string;
  location: string;
  assignee: number | null;
  due: string;
  created: string;
  notes: Note[];
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
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: {
    class: 'flex flex-col bg-white text-slate-900',
    '(document:keydown.escape)': 'onEscape()',
  },
})
export default class TaskManagementAdminComponent {
  readonly technicians: Technician[] = [
    { id: 1, name: 'Jan de Vries', initials: 'JV', color: 'bg-blue-600', available: true },
    { id: 2, name: 'Sara Ahmed', initials: 'SA', color: 'bg-emerald-600', available: true },
    { id: 3, name: 'Thomas Bakker', initials: 'TB', color: 'bg-amber-600', available: false },
    { id: 4, name: 'Lisa Chen', initials: 'LC', color: 'bg-violet-600', available: true },
    { id: 5, name: 'Mark Jansen', initials: 'MJ', color: 'bg-rose-600', available: true },
  ];

  tasks = signal<Task[]>([
    {
      id: 1,
      title: 'Replace broken harddisk',
      description:
        'Failed disk in Bay 3 of backup-srv-07 at Rack 123. Replace with Seagate Exos X18 (ST16000NM000J, 16 TB). The RAID controller shows the drive as failed since yesterday evening.',
      status: 'In Progress',
      priority: 'Critical',
      category: 'Hardware',
      location: 'DC Amsterdam-West · Rack 123',
      assignee: 1,
      due: '2026-03-20',
      created: '2026-03-15',
      notes: [
        {
          author: 1,
          text: 'Arrived at rack. Disk bay 3 LED is solid red. Starting replacement procedure.',
          time: '2 hours ago',
        },
        {
          author: null,
          text: 'Spare disk is available in storage room B, shelf 3. Serial: ZLR1N5JY.',
          time: '5 hours ago',
        },
      ],
    },
    {
      id: 2,
      title: 'Check cooling unit — Row 5',
      description:
        'Temperature sensors in Row 5 are reporting 2°C above normal baseline. Inspect the cooling unit for potential blockage or fan failure.',
      status: 'Ready',
      priority: 'High',
      category: 'Cooling',
      location: 'DC Amsterdam-West · Hall A, Row 5',
      assignee: null,
      due: '2026-03-21',
      created: '2026-03-17',
      notes: [
        {
          author: null,
          text: 'Monitoring dashboard shows temps rising over the past 48h. Not yet critical but trending up.',
          time: '1 day ago',
        },
      ],
    },
    {
      id: 3,
      title: 'Inspect PDU — Hall A',
      description:
        'Routine quarterly inspection of the PDU in Hall A. Check all breakers, verify load balancing, and ensure no burnt contacts.',
      status: 'Ready',
      priority: 'Medium',
      category: 'Power',
      location: 'DC Amsterdam-West · Hall A',
      assignee: 2,
      due: '2026-03-25',
      created: '2026-03-16',
      notes: [],
    },
    {
      id: 4,
      title: 'Replace network switch — Rack 87',
      description:
        'The Cisco Nexus switch in Rack 87 has intermittent port failures on ports 24-28. Replace with the new Arista unit from stock.',
      status: 'In Progress',
      priority: 'High',
      category: 'Network',
      location: 'DC Amsterdam-West · Rack 87',
      assignee: 4,
      due: '2026-03-19',
      created: '2026-03-14',
      notes: [
        {
          author: 4,
          text: 'Migration window confirmed with NOC for tonight 22:00–02:00. Pre-staging the replacement switch now.',
          time: '3 hours ago',
        },
        {
          author: null,
          text: 'NOC has been notified. Maintenance window approved.',
          time: '1 day ago',
        },
      ],
    },
    {
      id: 5,
      title: 'Firmware update — UPS units Hall B',
      description:
        'Apply firmware v4.2.1 to all three Eaton UPS units in Hall B. Requires sequential update — do not update all at once.',
      status: 'Review',
      priority: 'Medium',
      category: 'Power',
      location: 'DC Amsterdam-West · Hall B',
      assignee: 3,
      due: '2026-03-22',
      created: '2026-03-13',
      notes: [
        {
          author: 3,
          text: 'UPS-1 and UPS-2 updated successfully. UPS-3 scheduled for tomorrow morning. All readings normal after update.',
          time: '6 hours ago',
        },
      ],
    },
    {
      id: 6,
      title: 'Install additional cameras — Entrance B',
      description:
        'Mount two new security cameras at Entrance B as per the security audit recommendations. Cabling is already in place.',
      status: 'Blocked',
      priority: 'Low',
      category: 'Security',
      location: 'DC Amsterdam-West · Entrance B',
      assignee: 5,
      due: '2026-03-28',
      created: '2026-03-10',
      notes: [
        {
          author: 5,
          text: 'Cameras arrived but mounting brackets are the wrong model. Waiting for replacement brackets from supplier.',
          time: '2 days ago',
        },
        { author: null, text: 'Supplier confirmed new brackets ship Monday.', time: '1 day ago' },
      ],
    },
    {
      id: 7,
      title: 'Decommission server DB-14',
      description:
        'Server DB-14 in Rack 45 has been migrated to new hardware. Wipe disks, remove from rack, and update asset inventory.',
      status: 'Done',
      priority: 'Low',
      category: 'Hardware',
      location: 'DC Amsterdam-West · Rack 45',
      assignee: 1,
      due: '2026-03-17',
      created: '2026-03-08',
      notes: [
        {
          author: 1,
          text: 'Disks wiped with DBAN (3-pass). Server removed from rack and placed in decommission staging. Asset inventory updated.',
          time: '1 day ago',
        },
      ],
    },
    {
      id: 8,
      title: 'Repair cable management — Rack 92',
      description:
        'Cables in Rack 92 are obstructing airflow. Re-route and zip-tie all patch cables. Replace any damaged cables.',
      status: 'In Progress',
      priority: 'Medium',
      category: 'Hardware',
      location: 'DC Amsterdam-West · Rack 92',
      assignee: 2,
      due: '2026-03-23',
      created: '2026-03-16',
      notes: [],
    },
  ]);

  readonly statusStyles: Record<string, StatusStyle> = {
    Ready: {
      bg: 'bg-slate-100',
      text: 'text-slate-600',
      dot: 'bg-slate-400',
      kanbanAccent: 'bg-slate-400',
      kanbanBorder: 'border-slate-200',
    },
    'In Progress': {
      bg: 'bg-indigo-50',
      text: 'text-indigo-700',
      dot: 'bg-indigo-500',
      kanbanAccent: 'bg-indigo-500',
      kanbanBorder: 'border-indigo-200',
    },
    Review: {
      bg: 'bg-amber-50',
      text: 'text-amber-700',
      dot: 'bg-amber-500',
      kanbanAccent: 'bg-amber-500',
      kanbanBorder: 'border-amber-200',
    },
    Blocked: {
      bg: 'bg-red-50',
      text: 'text-red-700',
      dot: 'bg-red-500',
      kanbanAccent: 'bg-red-500',
      kanbanBorder: 'border-red-200',
    },
    Done: {
      bg: 'bg-emerald-50',
      text: 'text-emerald-700',
      dot: 'bg-emerald-500',
      kanbanAccent: 'bg-emerald-500',
      kanbanBorder: 'border-emerald-200',
    },
  };

  readonly priorityStyles: Record<string, PriorityStyle> = {
    Critical: {
      bg: 'bg-red-50',
      text: 'text-red-700',
      dot: 'bg-red-500',
      ring: 'ring-red-200/80',
    },
    High: {
      bg: 'bg-orange-50',
      text: 'text-orange-700',
      dot: 'bg-orange-500',
      ring: 'ring-orange-200/80',
    },
    Medium: {
      bg: 'bg-yellow-50',
      text: 'text-yellow-700',
      dot: 'bg-yellow-400',
      ring: 'ring-yellow-200/80',
    },
    Low: {
      bg: 'bg-slate-100',
      text: 'text-slate-500',
      dot: 'bg-slate-400',
      ring: 'ring-slate-200/80',
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

  readonly kanbanColumns = ['Ready', 'In Progress', 'Review', 'Blocked', 'Done'];

  readonly priorities = ['Critical', 'High', 'Medium', 'Low'];

  readonly taskCategories = ['Hardware', 'Network', 'Cooling', 'Power', 'Security', 'Other'];

  private readonly dateLocale = 'en-US';

  private readonly taskIdBase = 2890;

  currentView = signal<'list' | 'kanban'>('list');

  searchQuery = signal('');

  statusFilter = signal('all');

  priorityFilter = signal('all');

  categoryFilter = signal('all');

  selectedTasks = signal<Set<number>>(new Set());

  detailTaskId = signal<number | null>(null);

  editingTaskId = signal<number | null | undefined>(undefined);

  editFormTitle = signal('');

  editFormDescription = signal('');

  editFormStatus = signal('Ready');

  editFormPriority = signal('Medium');

  editFormCategory = signal('Hardware');

  editFormDue = signal('');

  editFormLocation = signal('');

  editFormAssignee = signal<number | null>(null);

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

  editModalTitle = computed(() => (this.editingTaskId() !== null ? 'Edit task' : 'New task'));

  readonly detailSheetEl = viewChild<ElementRef<NlddSheet>>('detailSheetEl');

  readonly editModalEl = viewChild<ElementRef<NlddSheet>>('editModalEl');

  getTech(id: number | null): Technician | null {
    return this.technicians.find((t) => t.id === id) ?? null;
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

  taskDisplayId(task: Task): string {
    return `T-${this.taskIdBase + task.id}`;
  }

  isSelected(id: number): boolean {
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

  toggleSelection(id: number, checked: boolean): void {
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

  openDetail(id: number): void {
    this.detailTaskId.set(id);
    this.detailSheetEl()?.nativeElement.show();
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

  openEditModal(taskId: number | null): void {
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
    this.editModalEl()?.nativeElement.show();
  }

  closeEditModal(): void {
    this.editModalEl()?.nativeElement.hide();
    this.editingTaskId.set(undefined);
  }

  saveTask(): void {
    const title = this.editFormTitle().trim();
    if (!title) return;

    const data = {
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
    if (editingId !== null && editingId !== undefined) {
      this.tasks.update((tasks) => tasks.map((t) => (t.id === editingId ? { ...t, ...data } : t)));
      this.showToast('Task updated');
    } else {
      const newTask: Task = {
        id: Date.now(),
        ...data,
        created: new Date().toISOString().split('T')[0],
        notes: [],
      };
      this.tasks.update((tasks) => [...tasks, newTask]);
      this.showToast('Task created');
    }

    this.editModalEl()?.nativeElement.hide();
    this.editingTaskId.set(undefined);
  }

  addNote(): void {
    const text = this.newNoteText().trim();
    if (!text) return;
    const id = this.detailTaskId();
    if (id === null) return;
    this.tasks.update((tasks) =>
      tasks.map((t) =>
        t.id === id ? { ...t, notes: [{ author: null, text, time: 'Just now' }, ...t.notes] } : t,
      ),
    );
    this.newNoteText.set('');
    this.showToast('Note added');
  }

  showToast(msg: string): void {
    this.toastMessage.set(msg);
    clearTimeout(this.toastTimeout);
    this.toastTimeout = window.setTimeout(() => {
      this.toastMessage.set(null);
    }, 2000);
  }

  noteAuthor(note: Note): { name: string; tech: Technician | null } {
    const tech = note.author !== null ? this.getTech(note.author) : null;
    return { name: tech ? tech.name : 'Admin', tech };
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
    return `cursor-pointer rounded-xl border ${s.kanbanBorder} bg-white p-3.5 hover:shadow-md hover:shadow-slate-200/80 transition-shadow`;
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
    `inline-flex ${size} items-center justify-center rounded-full bg-slate-200 text-slate-500 font-medium shrink-0`;

  shortDate(str: string | null): string {
    if (!str) return '';
    return new Date(`${str}T00:00:00`).toLocaleDateString(this.dateLocale, {
      month: 'short',
      day: 'numeric',
    });
  }
}
