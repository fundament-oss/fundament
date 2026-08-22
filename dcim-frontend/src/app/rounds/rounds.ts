import {
  ChangeDetectionStrategy,
  Component,
  computed,
  CUSTOM_ELEMENTS_SCHEMA,
  effect,
  inject,
  OnDestroy,
  OnInit,
  TemplateRef,
  viewChild,
} from '@angular/core';
import { NgTemplateOutlet } from '@angular/common';
import { toSignal } from '@angular/core/rxjs-interop';
import { ActivatedRoute, convertToParamMap, Router } from '@angular/router';
import SecondaryNavService from '../shell/secondary-nav.service';
import RoundsService from './rounds.service';
import { dayLabel, Round } from './round';
import { personSlug, ROUNDS_PATH } from './round-views';
import TaskManagementTechnicianComponent from '../task-management-technician/task-management-technician';
import { TASKS_PATH } from '../tasks/task-views';

/**
 * The rounds of every technician, and one of them opened.
 *
 * Read-only, all of it. A planner looking at somebody else's round has no
 * business ticking off work they did not do; that belongs to whoever stood in
 * the aisle. Changing the work itself happens in Tasks.
 */
@Component({
  selector: 'app-rounds',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  imports: [NgTemplateOutlet, TaskManagementTechnicianComponent],
  templateUrl: './rounds.html',
})
export default class RoundsComponent implements OnInit, OnDestroy {
  protected readonly rounds = inject(RoundsService);

  private readonly secondaryNav = inject(SecondaryNavService);

  private readonly router = inject(Router);

  private readonly route = inject(ActivatedRoute);

  private readonly navTemplate = viewChild<TemplateRef<unknown>>('secondaryNav');

  private readonly params = toSignal(this.route.paramMap, {
    initialValue: convertToParamMap({}),
  });

  protected readonly tasksPath = TASKS_PATH;

  private readonly dateLocale = 'en-US';

  constructor() {
    // No rounds means no menu: two empty panes beside each other say the same
    // nothing twice, so the list takes the whole width and says it there.
    effect(() => {
      const template = this.navTemplate();
      if (!template) return;
      if (this.rounds.rounds().length) this.secondaryNav.set(template);
      else this.secondaryNav.clear(template);
    });
  }

  ngOnInit(): void {
    this.rounds.load();
  }

  ngOnDestroy(): void {
    const template = this.navTemplate();
    if (template) this.secondaryNav.clear(template);
  }

  /** The round the address names, if it names one that exists. */
  protected readonly selected = computed<Round | null>(() => {
    const params = this.params();
    const person = params.get('person');
    const datacenter = params.get('datacenter');
    if (!person || !datacenter) return null;
    const day = params.get('day');
    return (
      this.rounds
        .rounds()
        .find(
          (round) =>
            (personSlug(round.personName) === person || round.personId === person) &&
            round.datacenter === datacenter &&
            (day ? round.day === day : this.rounds.isToday(round.day)),
        ) ?? null
    );
  });

  protected isSelected(round: Round): boolean {
    return this.selected()?.key === round.key;
  }

  /** Today's round needs no date in its address; every other one does. */
  protected roundPath(round: Round): string {
    const base = `${ROUNDS_PATH}/${personSlug(round.personName)}/${round.datacenter}`;
    return this.rounds.isToday(round.day) ? base : `${base}/${round.day}`;
  }

  protected goToRound(event: Event, round: Round): void {
    event.preventDefault();
    this.router.navigateByUrl(this.roundPath(round));
  }

  protected goToRounds(): void {
    this.router.navigateByUrl(ROUNDS_PATH);
  }

  protected goToTasks(event: Event): void {
    event.preventDefault();
    this.router.navigateByUrl(TASKS_PATH);
  }

  protected goToInbox(event: Event): void {
    event.preventDefault();
    this.router.navigateByUrl(`${TASKS_PATH}/inbox`);
  }

  protected dayLabel(day: string): string {
    return dayLabel(day, this.rounds.todayISO(), this.dateLocale);
  }

  /**
   * How far a round has come. A round that has not started yet says how much
   * work it holds instead: nought out of twenty-six reads as falling behind,
   * where nothing has gone wrong at all.
   */
  protected progressLabel(round: Round): string {
    if (!this.rounds.isToday(round.day)) {
      return `${round.tasks.length} ${round.tasks.length === 1 ? 'task' : 'tasks'}`;
    }
    const progress = this.rounds.taskProgress(round);
    return `${progress.done}/${progress.total}`;
  }

  protected readonly unplacedLabel = computed(() => {
    const count = this.rounds.unplaced().length;
    return `${count} ${count === 1 ? 'task is' : 'tasks are'} in no round`;
  });
}
