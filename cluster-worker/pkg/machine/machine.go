// Package machine describes a state machine's allowed edges as data.
//
// The table is the reviewable copy of a machine: Validate rejects a table with
// a duplicate edge, an unreachable state or a non-terminal state with no exit,
// and Mermaid renders the same edges for the docs. Enforcement of a transition
// stays in SQL: every write that moves a row carries the state it expects, so
// two processes racing over one row cannot both win.
package machine

import (
	"fmt"
	"slices"
	"strings"
)

// Transition is one allowed edge: in state From, Event leads to To.
type Transition[State ~string, Event ~string] struct {
	From  State
	Event Event
	To    State
}

// Table is the set of allowed edges of one machine.
type Table[State ~string, Event ~string] struct {
	name        string
	initial     State
	transitions []Transition[State, Event]
	terminals   []State
}

// New starts a table whose machine starts in initial.
func New[State ~string, Event ~string](name string, initial State, transitions ...Transition[State, Event]) Table[State, Event] {
	return Table[State, Event]{name: name, initial: initial, transitions: transitions}
}

// Terminal marks states the machine can end in: they must have no exit, and
// every other state must have one.
func (t Table[State, Event]) Terminal(states ...State) Table[State, Event] {
	t.terminals = append(t.terminals, states...)
	return t
}

// Must panics when the table is invalid. A malformed table is a programming
// error, so it fails at start-up rather than on the first transition.
func Must[State ~string, Event ~string](t Table[State, Event]) Table[State, Event] {
	if err := t.Validate(); err != nil {
		panic(fmt.Sprintf("machine %q: %v", t.name, err))
	}
	return t
}

// Apply returns where an event fired in from leads, and whether that edge is
// declared.
func (t Table[State, Event]) Apply(from State, event Event) (State, bool) {
	for _, tr := range t.transitions {
		if tr.From == from && tr.Event == event {
			return tr.To, true
		}
	}
	var zero State
	return zero, false
}

// Validate rejects a table that cannot describe a working machine:
// an edge declared twice for the same state and event, a state the machine can
// never reach, a terminal state with an exit, or a non-terminal state without
// one. A declared terminal is a state like any other: no edge leading to it is
// as broken as one leading nowhere.
func (t Table[State, Event]) Validate() error {
	if len(t.transitions) == 0 {
		return fmt.Errorf("no transitions")
	}

	seen := make(map[[2]string]struct{}, len(t.transitions))
	exits := make(map[State]int, len(t.transitions))
	states := map[State]struct{}{t.initial: {}}
	for _, tr := range t.transitions {
		key := [2]string{string(tr.From), string(tr.Event)}
		if _, dup := seen[key]; dup {
			return fmt.Errorf("duplicate edge %q on event %q", tr.From, tr.Event)
		}
		seen[key] = struct{}{}
		exits[tr.From]++
		states[tr.From] = struct{}{}
		states[tr.To] = struct{}{}
	}

	terminal := make(map[State]struct{}, len(t.terminals))
	for _, s := range t.terminals {
		terminal[s] = struct{}{}
		states[s] = struct{}{}
		if exits[s] > 0 {
			return fmt.Errorf("terminal state %q has an exit", s)
		}
	}

	// The checks below report the first offender, so walk the states in a fixed
	// order: a table with more than one defect must name the same one every run.
	ordered := make([]State, 0, len(states))
	for s := range states {
		ordered = append(ordered, s)
	}
	slices.Sort(ordered)

	for _, s := range ordered {
		if _, isTerminal := terminal[s]; !isTerminal && exits[s] == 0 {
			return fmt.Errorf("state %q has no exit", s)
		}
	}

	reachable := map[State]struct{}{t.initial: {}}
	queue := []State{t.initial}
	for len(queue) > 0 {
		from := queue[0]
		queue = queue[1:]
		for _, tr := range t.transitions {
			if tr.From != from {
				continue
			}
			if _, ok := reachable[tr.To]; ok {
				continue
			}
			reachable[tr.To] = struct{}{}
			queue = append(queue, tr.To)
		}
	}
	for _, s := range ordered {
		if _, ok := reachable[s]; !ok {
			return fmt.Errorf("state %q is unreachable from %q", s, t.initial)
		}
	}

	return nil
}

// Mermaid renders the table as a Mermaid state diagram.
func (t Table[State, Event]) Mermaid() string {
	var b strings.Builder
	b.WriteString("stateDiagram-v2\n")

	ids := make(map[State]string)
	id := func(s State) string {
		if v, ok := ids[s]; ok {
			return v
		}
		v := fmt.Sprintf("s%d", len(ids))
		ids[s] = v
		fmt.Fprintf(&b, "    state %q as %s\n", string(s), v)
		return v
	}

	fmt.Fprintf(&b, "    [*] --> %s\n", id(t.initial))
	for _, tr := range t.transitions {
		fmt.Fprintf(&b, "    %s --> %s : %s\n", id(tr.From), id(tr.To), string(tr.Event))
	}
	for _, s := range t.terminals {
		fmt.Fprintf(&b, "    %s --> [*]\n", id(s))
	}

	return b.String()
}
