import { TaskData } from '../task-management/task-api.service';
import { taskTags } from '../tasks/task-tags';

/**
 * A round is one person, one data center, one day.
 *
 * It is derived, not stored. Nothing is written when one is composed, and a
 * round only exists for as long as there is work that answers to all three.
 * That definition is what makes the gather step honest: you collect material
 * once and then walk, so the walk has to be one trip to one place.
 */
export interface Round {
  /** person + data center + day, the address the round is reached at. */
  key: string;
  personId: string;
  personName: string;
  datacenter: string;
  /** ISO date. Today's round carries today, whatever the tasks in it are due. */
  day: string;
  tasks: TaskData[];
}

/** Why a task is in no round at all. In the order you would fix them. */
export type Gap = 'assignee' | 'due' | 'location';

export interface UnplacedTask {
  task: TaskData;
  gap: Gap;
}

export const roundKey = (personId: string, datacenter: string, day: string): string =>
  `${personId}|${datacenter}|${day}`;

/**
 * The data center a task stands in, read from the tag that names a place.
 *
 * A tag is a path, `AMS1/R01-3`, so the first segment is the site and the rest
 * is detail within it. Which first segments count as a site comes from the list
 * of data centers rather than from the shape of the tag, because `network` is
 * not a place.
 */
export function taskDatacenter(task: TaskData, datacenters: string[]): string {
  const known = new Set(datacenters);
  const site = taskTags(task).find((tag) => known.has(tag.split('/')[0]));
  return site ? site.split('/')[0] : '';
}

/**
 * Which day's round a task belongs to. Overdue work joins today rather than
 * staying behind on the day it was due: it is still to be done, and today is
 * when you are walking. Its due date is not rewritten for that — the round is
 * derived, and rewriting would lose that the work ran late.
 *
 * Work without a due date joins nothing. Sweeping it into today would make
 * today's round unbounded and would collect material for work nobody planned.
 */
export function roundDay(task: TaskData, today: string): string | null {
  if (!task.due) return null;
  return task.due <= today ? today : task.due;
}

/** Whether this task is still to be walked at all. */
const isOpen = (task: TaskData): boolean => task.status !== 'Done';

/**
 * The order you walk a round in.
 *
 * Urgent first, because urgent is what justifies a detour. Then by the path the
 * tag names, so `R01-3` comes before `R02-1` and you walk the aisle once rather
 * than crossing the hall between every task. Creation date breaks what is left.
 *
 * Priority below urgent deliberately does not sort. Standing in that aisle
 * already, you do both, and a high-priority task three rows away is not worth
 * the walk. It reads as a label instead.
 */
export function sortRoundTasks(tasks: TaskData[]): TaskData[] {
  const place = (task: TaskData): string =>
    taskTags(task).find((tag) => tag.includes('/')) ?? '￿';
  return [...tasks].sort((a, b) => {
    const urgency = Number(b.priority === 'Urgent') - Number(a.priority === 'Urgent');
    if (urgency !== 0) return urgency;
    const path = place(a).localeCompare(place(b));
    if (path !== 0) return path;
    return a.created.localeCompare(b.created);
  });
}

/**
 * Every round there is, today's first and the rest by date after it.
 *
 * A day three months out is shown rather than hidden. It is almost always a
 * wrong due date, and a list of rounds is where you notice that.
 */
export function buildRounds(
  tasks: TaskData[],
  personName: (id: string) => string,
  datacenters: string[],
  today: string,
): Round[] {
  const byKey = new Map<string, Round>();

  tasks
    .filter((task) => isOpen(task) && !!task.assignee)
    .forEach((task) => {
      const personId = task.assignee as string;
      const day = roundDay(task, today);
      const datacenter = taskDatacenter(task, datacenters);
      if (!day || !datacenter) return;

      const key = roundKey(personId, datacenter, day);
      const round = byKey.get(key);
      if (round) round.tasks.push(task);
      else {
        byKey.set(key, {
          key,
          personId,
          personName: personName(personId),
          datacenter,
          day,
          tasks: [task],
        });
      }
    });

  const rounds = [...byKey.values()].map((round) => ({
    ...round,
    tasks: sortRoundTasks(round.tasks),
  }));

  // Within a day the round carrying the most urgent work stands on top, then
  // alphabetically, so what wants attention is always above what does not.
  return rounds.sort((a, b) => {
    if (a.day !== b.day) return a.day.localeCompare(b.day);
    const urgency =
      Number(b.tasks.some((t) => t.priority === 'Urgent')) -
      Number(a.tasks.some((t) => t.priority === 'Urgent'));
    if (urgency !== 0) return urgency;
    const person = a.personName.localeCompare(b.personName);
    return person !== 0 ? person : a.datacenter.localeCompare(b.datacenter);
  });
}

/**
 * Work that answers to none of the three questions a round asks: who, when and
 * where. It is the planning that has not happened yet, which is exactly the
 * work that is not going to get done.
 */
export function unplacedTasks(tasks: TaskData[], datacenters: string[]): UnplacedTask[] {
  return tasks.filter(isOpen).flatMap<UnplacedTask>((task) => {
    if (!task.assignee) return [{ task, gap: 'assignee' }];
    if (!task.due) return [{ task, gap: 'due' }];
    if (!taskDatacenter(task, datacenters)) return [{ task, gap: 'location' }];
    return [];
  });
}

/** Rounds grouped by their day, in the order they should be read. */
export function roundsByDay(rounds: Round[]): { day: string; rounds: Round[] }[] {
  const days: { day: string; rounds: Round[] }[] = [];
  rounds.forEach((round) => {
    const group = days.find((d) => d.day === round.day);
    if (group) group.rounds.push(round);
    else days.push({ day: round.day, rounds: [round] });
  });
  return days;
}

/** A day as you read it: today and tomorrow by name, the rest by date. */
export function dayLabel(day: string, today: string, locale = 'en-US'): string {
  if (day === today) return 'Today';
  const next = new Date(`${today}T00:00:00`);
  next.setDate(next.getDate() + 1);
  if (day === next.toISOString().slice(0, 10)) return 'Tomorrow';
  return new Date(`${day}T00:00:00`).toLocaleDateString(locale, {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
  });
}
