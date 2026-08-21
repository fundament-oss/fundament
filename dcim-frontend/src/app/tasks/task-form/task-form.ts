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
import { firstValueFrom, Observable } from 'rxjs';
import TaskApiService, {
  TaskData,
  TaskInput,
  TaskPriorityLabel,
  TaskStatusLabel,
} from '../../task-management/task-api.service';
import UserApiService, { RosterUser } from '../../task-management/user-api.service';
import TaskAttentionService from '../task-attention.service';
import { taskTags } from '../task-tags';
import DatacenterListService from '../../datacenters/datacenter-list.service';
import PlacementApiService, { RackOption } from '../../inventory/placement-api.service';
import ToastService from '../../shared/toast.service';
import connectErrorMessage from '../../../connect/error';

/**
 * The fields of one task, without anything around them.
 *
 * It carries no sheet and no bar of its own, because it is shown in two
 * places: as the sheet the add button in the bar opens, and as the second view
 * of the task you already have open, where the way back is a back button
 * rather than a dismiss. What happens after saving is the host's call, which
 * is what `saved` is for.
 */
@Component({
  selector: 'app-task-form',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './task-form.html',
})
export default class TaskFormComponent {
  /** The task being edited. Without an id it is a new one. */
  readonly task = input.required<Partial<TaskData>>();

  /** Saved and written away. The host decides where to go next. */
  readonly saved = output<void>();

  private readonly taskApi = inject(TaskApiService);

  private readonly userApi = inject(UserApiService);

  private readonly attention = inject(TaskAttentionService);

  private readonly toast = inject(ToastService);

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

  private readonly titleInput = viewChild<ElementRef<HTMLElement>>('editTitleInput');

  constructor() {
    effect(() => {
      const task = this.task();
      this.editFormTitle.set(task.title ?? '');
      this.editFormDescription.set(task.description ?? '');
      this.editFormStatus.set(task.status ?? 'To do');
      this.editFormPriority.set(task.priority ?? 'None');
      // One list, the same one the menu is built from: a place a task names is
      // a tag like any other, and a task that carries it as a location text
      // reads as the path it describes.
      this.editFormTags.set(taskTags({ tags: task.tags ?? [], location: task.location ?? '' }));
      this.editFormDue.set(task.due ?? '');
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

  protected save(): void {
    this.saveAttempted.set(true);
    const title = this.editFormTitle().trim();
    if (!title) {
      this.titleInput()?.nativeElement.focus();
      return;
    }
    const task = this.task();

    const values: TaskInput = {
      title,
      description: this.editFormDescription().trim(),
      status: this.editFormStatus(),
      priority: this.editFormPriority(),
      tags: this.editFormTags(),
      due: this.editFormDue(),
      // A place used to live in a field of its own, which produced three
      // spellings of one rack. It is a tag now, so saving is the migration:
      // whatever the location said stands among the tags and the old field is
      // cleared.
      location: '',
      assignee: this.editFormAssignee(),
    };

    let request: Observable<unknown>;
    if (!task.id) {
      request = this.taskApi.createTask(values);
    } else {
      // Only what this form changed is sent. Posting the whole form writes back
      // every field as the form last saw it, which silently reverts what
      // somebody else changed while it was open.
      const patch = TaskApiService.changedFields(task as TaskData, values);
      if (Object.keys(patch).length === 0) {
        this.saved.emit();
        return;
      }
      request = this.taskApi.updateTask(task.id, patch);
    }

    firstValueFrom(request)
      .then(() => {
        this.attention.markChanged();
        this.saved.emit();
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
