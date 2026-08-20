import {
  ChangeDetectionStrategy,
  Component,
  computed,
  CUSTOM_ELEMENTS_SCHEMA,
  effect,
  ElementRef,
  inject,
  signal,
  viewChild,
} from '@angular/core';
import { firstValueFrom, Observable } from 'rxjs';
import TaskApiService, {
  TaskData,
  TaskInput,
  TaskPriorityLabel,
  TaskStatusLabel,
} from '../../task-management/task-api.service';
import UserApiService, { RosterUser } from '../../task-management/user-api.service';
import TaskAttentionService from '../task-attention.service';
import DatacenterListService from '../../datacenters/datacenter-list.service';
import PlacementApiService, { RackOption } from '../../inventory/placement-api.service';
import OverlayService from '../../shell/overlay.service';
import ToastService from '../../shared/toast.service';
import connectErrorMessage from '../../../connect/error';

interface NativeElementRef {
  nativeElement: { show?: () => void; hide?: () => void; focus?: () => void };
}

/**
 * The form for one task, held by the shell rather than by the tasks page.
 *
 * Writing down what has to happen is the thing you do while looking at
 * something else: a rack that is full, an asset that is broken. So the form
 * opens from the add button in the bar over whatever page you are on, and the
 * page it belongs to does not have to be there.
 */
@Component({
  selector: 'app-task-sheet',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './task-sheet.html',
})
export default class TaskSheetComponent {
  private readonly taskApi = inject(TaskApiService);

  private readonly userApi = inject(UserApiService);

  private readonly attention = inject(TaskAttentionService);

  private readonly toast = inject(ToastService);

  protected readonly overlays = inject(OverlayService);

  protected readonly form = this.overlays.taskSheet;

  protected readonly title = computed(() => (this.form()?.id ? 'Edit task' : 'New task'));

  protected readonly technicians = signal<RosterUser[]>([]);

  private readonly datacenterList = inject(DatacenterListService);

  private readonly datacenters = this.datacenterList.datacenters;

  private readonly placementApi = inject(PlacementApiService);

  /** Every rack, to offer as a tag under the data center it stands in. */
  private readonly racks = signal<RackOption[]>([]);

  /**
   * What the tag field offers: the places first, then the tags other tasks
   * already carry.
   *
   * A data center and a rack are fixed tags, not ones somebody types: they
   * exist whether a task mentions them or not, so they come from the lists of
   * data centers and racks. A rack is written as a path under its hall
   * (`AMS1/R01-3`), which is what makes filtering on the hall find it. Anything
   * else is free, which is why the two are merged instead of kept apart.
   */
  protected readonly knownTags = computed(() => {
    const sites = this.datacenters().map((dc) => dc.name);
    const racks = this.racks().map((rack) => `${rack.datacenter}/${rack.name}`);
    const fixed = [...sites, ...racks];
    const used = this.attention.tags().filter((tag) => !fixed.includes(tag));
    return [...fixed, ...used];
  });

  protected readonly editFormTitle = signal('');

  protected readonly editFormDescription = signal('');

  protected readonly editFormStatus = signal<TaskStatusLabel>('To do');

  protected readonly editFormPriority = signal<TaskPriorityLabel>('None');

  protected readonly editFormTags = signal<string[]>([]);

  protected readonly editFormDue = signal('');

  /**
   * Where the work is. No field asks for it any more: a task is written down
   * beside the thing it is about, and typing the place again by hand produced
   * three spellings of one rack. Kept and sent back unchanged, so editing a
   * task does not wipe what is already there, and so the section menu can keep
   * grouping by the data center it names.
   */
  protected readonly editFormLocation = signal('');

  protected readonly editFormAssignee = signal<string | null>(null);

  /** The whole choice in view, as buttons: three states and five priorities
   *  are short enough to read at a glance, where a dropdown hides them. */
  protected readonly STATUSES: TaskStatusLabel[] = ['To do', 'Doing', 'Done'];

  /** Low to high, so the row reads as a scale and lands where the status row
   *  does: nothing set on the left, most pressing on the right. */
  protected readonly PRIORITIES: TaskPriorityLabel[] = ['None', 'Low', 'Medium', 'High', 'Urgent'];

  /** Set by pressing Save: an empty title is unfinished, not wrong. */
  protected readonly saveAttempted = signal(false);

  protected readonly titleInvalid = computed(
    () => this.saveAttempted() && !this.editFormTitle().trim(),
  );

  private readonly sheetEl = viewChild<NativeElementRef>('taskSheet');

  private readonly titleInput = viewChild<ElementRef<HTMLElement>>('editTitleInput');

  constructor() {
    effect(() => {
      const el = this.sheetEl()?.nativeElement;
      if (this.form() !== null) el?.show?.();
      else el?.hide?.();
    });
    effect(() => {
      const task = this.form();
      if (!task) return;
      this.editFormTitle.set(task.title ?? '');
      this.editFormDescription.set(task.description ?? '');
      this.editFormStatus.set(task.status ?? 'To do');
      this.editFormPriority.set(task.priority ?? 'None');
      this.editFormTags.set(task.tags ? [...task.tags] : []);
      this.editFormDue.set(task.due ?? '');
      this.editFormLocation.set(task.location ?? '');
      this.editFormAssignee.set(task.assignee ?? null);
      this.saveAttempted.set(false);
      if (this.technicians().length === 0) this.loadTechnicians();
      if (this.attention.tags().length === 0) this.attention.refresh();
      if (this.datacenters().length === 0) this.datacenterList.load();
      if (this.racks().length === 0) this.loadRacks();
      // The sheet and Lit's shadow render finish on microtasks, so the field
      // is only there to focus a tick later.
      setTimeout(() => this.titleInput()?.nativeElement.focus());
    });
  }

  /** One at a time: unpicking the current one leaves it as it was. */
  protected onStatusToggle(status: TaskStatusLabel, selected: boolean): void {
    if (selected) this.editFormStatus.set(status);
  }

  protected onPriorityToggle(priority: TaskPriorityLabel, selected: boolean): void {
    if (selected) this.editFormPriority.set(priority);
  }

  protected close(): void {
    this.overlays.closeTask();
  }

  protected save(): void {
    this.saveAttempted.set(true);
    const title = this.editFormTitle().trim();
    if (!title) {
      this.titleInput()?.nativeElement.focus();
      return;
    }
    const task = this.form();
    if (!task) return;

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

    let request: Observable<unknown>;
    if (!task.id) {
      request = this.taskApi.createTask(input);
    } else {
      // Only what this form changed is sent. Posting the whole form writes back
      // every field as the sheet last saw it, which silently reverts what
      // somebody else changed while it was open.
      const patch = TaskApiService.changedFields(task as TaskData, input);
      if (Object.keys(patch).length === 0) {
        this.overlays.closeTask();
        return;
      }
      request = this.taskApi.updateTask(task.id, patch);
    }

    firstValueFrom(request)
      .then(() => {
        this.attention.markChanged();
        this.overlays.closeTask();
      })
      .catch((err) => {
        // eslint-disable-next-line no-console
        console.error(connectErrorMessage(err));
        this.toast.error('Could not save task');
      });
  }

  private loadRacks(): void {
    this.placementApi
      .listRackOptions()
      .then((racks) => this.racks.set(racks))
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }

  private loadTechnicians(): void {
    firstValueFrom(this.userApi.listUsers())
      .then((res) => this.technicians.set(res.users.map((u) => UserApiService.mapUser(u))))
      // eslint-disable-next-line no-console
      .catch((err) => console.error(connectErrorMessage(err)));
  }
}
