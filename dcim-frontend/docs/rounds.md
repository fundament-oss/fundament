# Rounds

The technician view shows a whole queue of work but hangs off a single task: you
open one task, press a button in its toolbar, and get five tasks you did not ask
for. The header says "DC Amsterdam-West" and "Work order WO-2241", and both are
hardcoded strings. The tasks under them come from two different data centers.

This plan gives that view a name, a place, and a definition.

## A round is one person, one data center, one day

A round holds the tasks assigned to that person, tagged with that data center,
that are overdue or due today. It is derived, not stored: there is no round
entity and nothing gets written when one is composed.

One person can have two rounds on the same day, one per data center. They are
listed under each other. Which trip to make first is the technician's call, not
the application's.

This definition is what makes the gather step honest. You collect material once
and then walk, so the walk has to be one trip to one place.

## Where it lives

`Rounds` is a main navigation item, under `Tasks`. Not a view toggle next to
Kanban: a round carries content that exists nowhere else (the gather checklist)
and a state of its own (where you are), which makes it a thing rather than a
grouping of tasks.

### The secondary sidebar lists rounds, grouped by day

Today at the top, the rest below it in date order. Nothing collapses. A round
three months out is almost always a wrong due date, and the sidebar is where you
notice it.

The day is a group heading: an `nldd-title size="5"` between spacers, above its
own list. That is the pattern the `Tags` heading in the tasks sidebar already
uses, and it keeps the heading out of the list's own keyboard order.

A row is an `nldd-text-cell` with the person as `text` and the data center as
`supporting-text`, and a cell to the right of it with progress as `7/26`. Plain
text, no progress circle. A round that has not started yet shows its task count
instead, because an empty circle reads as falling behind.

Within a day, the round with the most urgent work stands on top, then
alphabetically.

A round that is finished should stay in the list for the rest of its day, shown
as done, because dropping it hides that Sem was there this morning. **Not
built**: a round is derived from work that is still open, and telling a task
finished today from one finished last week needs a completion date the model
does not carry. Same field, same blocker as the history below.

At the bottom of the sidebar, `Earlier` opens past rounds in the main pane.
**Not built**, for the same reason.

Above the list, one line for work that is in no round: "5 tasks are in no round.
Sort that out in Inbox." A sentence with a link rather than a row in the menu:
the rows here are rounds, and this one leaves the section. One line rather than
three, with the reason per task in the list it opens.

### The main pane shows the round, read-only

The header becomes the person, the data center and the day. That replaces
`dcName` and `WO-2241`.

The walkthrough itself stays as it is: the gather step, the tasks with their
steps, one open card under the step the work stands on.

Its own layout is the design system's: an `nldd-simple-section` carries the
reading width and the padding where a Tailwind wrapper used to, and the round's
heading is an `nldd-title` with the data center, day and count in its subtitle
slot. The page's title bar collapses onto that title. Standing on its own the
view still brings a `<main>` of its own, because there is no page to lend it
one.

`Previous`, `Note` and `Done` are gone. A planner looking at somebody else's
round has no business ticking off work they did not do. Clicking through to
another step to read it stays, because that changes what you look at, not what
is there.

The way on lives under the card of the step it belongs to, not in a bar along
the bottom. A bar acts on "whatever is open", which is the same implicit
reference the note button had before it moved. Reading, that button is `Next
step` and it only moves what you are looking at; walking, it is `Done` and it
writes. Under the gather card too, because that is a step like any other. Back
is a click on any earlier row, so it needs no button of its own.

Primary, and at the ordinary size. It is the one thing to do under what you just
read, and no longer half of a Previous/Next pair where singling one out would
have said forward beats back.

There is no footer at all. Progress moved up to the heading, beside the name
rather than under it — `Daan Hofman 6/20`, with `AMS1 · Today · 4 tasks` as the
line below — and the gather step carries its own count the same way.

What that costs, and where: a bar pinned to the bottom is a target you hit with
gloves on without looking, and an inline button moves with the card and can sit
below a tall drawing. That is the technician's environment, which is parked, and
it is the one place where the bar may have to come back.

Progress counts the work and leaves the gather step out of it. Collecting
material is preparation for a round rather than part of it, and counting it puts
you at 1 of 21 for having picked up a screwdriver. It also keeps the number the
same as the one in the sidebar, which cannot know whether somebody ticked their
gather list: that lives in their browser.

With nothing selected, the pane says to pick a round. With no rounds at all,
there is no second pane: the list takes the whole width and carries an
`nldd-inline-dialog` saying nothing is planned, with a button to Tasks.

## A note belongs to a step, and hangs off the task

One place, directly under the task's header: the notes and the field to add one.
A note is about the task, it is filed against the task, and it is read there, so
nothing about a step is claimed and nothing has to be parsed back out of the text
later.

A `Note` is polymorphic (`entity_type` plus `entity_id`) and the enum stops at
`NOTE_ENTITY_TYPE_TASK`, so a step-scoped note would mean a new value and a
backend that accepts it. What is given up by not doing that: which step a remark
was about. A button under the step would have recorded it by naming the step in
the body, but that is structure written into prose, and it made two buttons out
of what reads as one thing.

Anyone may write one, including a planner reading a round. Authorship is not a
worry: `created_by` and `created_by_id` are resolved server-side from the
authenticated caller and cannot be supplied by the client.

Writing one happens in the list itself: the last row of the box is a field with
an `Add` beside it, so a new note appears right above where you typed it. That is
the shape the task's own detail already uses, it takes no more room than a button
would, and there is nothing to open first.

## Inside a round, the order is the walk

Urgent first, because urgent is what justifies a detour. Then by rack path, so
`R01-3` comes before `R02-1` and you walk the aisle once. Creation date breaks
the remaining ties.

Priority below urgent is a label, not an order. Standing in that aisle already,
you do both.

A task without steps counts as one step: it appears in the round and you tick it
off whole. Without that rule, a one-action job like replacing a filter would
silently drop out of the round for lack of a checklist.

The gather checklist is per round, and so is its checked state. Two rounds on one
day are two trips with two sets of material.

## Gaps belong in Inbox

The Inbox rule already covers two of the three: not done, and no assignee or no
due date (`tasks.ts:391`). Widen it with the third, no location. Inbox then means
one thing: this cannot enter a round yet.

Work without a due date does not get swept into today. That would make today's
round unbounded, would collect material for work nobody planned, and would hide
the planning gap instead of showing it.

An overdue task appears in today's round because the definition already says so.
Its due date is not rewritten. Rewriting it would lose that the work was late,
from a view that does not write.

## History is derived from completion — not built yet

Everything in this section waits on two fields. It is the only part of this plan
that cannot be written in the frontend alone.


A past round is the tasks that person completed on that day at that data center.

That needs a field the model does not have. `TaskData` carries `status`, `due`
and `created`, and nothing about finishing; the "Done by Sem Bakker" line in the
task list is the assignee, not whoever closed it (`tasks.ts:474`).

So add `completed` and `completed_by_id` to the task. Do not rename `due_date`
or `created`: the wire already says `due_date`, only the TypeScript interface
shortens it to `due`, and a due date is a day where a completion is a moment.
Reopening a task clears `completed`, or it stays in the history of a day it no
longer belongs to.

What this gives up: a task planned for Tuesday and finished on Wednesday shows on
Wednesday, and Tuesday never shows that it ran out. History says what was done,
not what was asked. Seeing that difference would mean freezing a round at
composition, which is the moment it stops being derived.

## From a task, a box instead of a button

The `Technician view` button in the detail toolbar goes. A toolbar button offers
an action on the task; what you want here is context.

A box at the bottom of the task detail says where the task sits: "Sem Bakker
walks this task as step 3 of 5 in his round in AMS1, today." Most of the time
that is the whole answer. A button next to it opens that round in a sheet,
read-only, so the planner keeps their place in the list.

A task that is in nobody's round says that instead. That is exactly when the box
is worth reading.

## Writing stays with the technician

Rounds is read-only for everyone. In this environment everyone may look at every
round.

Steps can only be ticked in the walkthrough, so the standalone route stays the
writable entry until there is a technician environment of its own — and it has
grown into the shape of one.

It brings its own chrome now: a bar with the product and the light/dark/system
switch, and a sidebar headed `My rounds` listing the rounds of whoever is logged
in, with the one being walked marked. No way back to a task list, because a
technician has none to go back to.

That also closed a gap this plan had and the code did not honour: the route used
to load every open task assigned to you, whatever its data center or day, so the
list held two sites at once and collecting material for it meant nothing. It
walks one round now, like every other view of one.

## Parked

- A Fundament Tech Round environment: the technician logs in and lands on their
  own round, no picker.
- Live updates while a planner has a round open. It is a snapshot until
  reloaded.
- Marking, inside a round, which tasks rolled over from yesterday.
- Roles and permissions.
- Freezing a round, needed only if planned-versus-done ever has to be visible.

## What this asks of the API

Two fields on a task: `completed` and `completed_by_id`.
