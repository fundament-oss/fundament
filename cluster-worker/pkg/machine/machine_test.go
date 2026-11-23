package machine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type state string

type event string

const (
	statePending   state = "pending"
	stateRunning   state = "running"
	stateCompleted state = "completed"
	stateFailed    state = "failed"

	eventStart event = "start"
	eventDone  event = "done"
	eventFail  event = "fail"
	eventFail2 event = "fail-again"
)

func validTable() Table[state, event] {
	return New("test",
		statePending,
		Transition[state, event]{From: statePending, Event: eventStart, To: stateRunning},
		Transition[state, event]{From: stateRunning, Event: eventDone, To: stateCompleted},
		Transition[state, event]{From: stateRunning, Event: eventFail, To: stateFailed},
		Transition[state, event]{From: stateRunning, Event: eventFail2, To: stateFailed},
	).Terminal(stateCompleted, stateFailed)
}

func TestApply(t *testing.T) {
	t.Parallel()

	table := validTable()

	to, ok := table.Apply(stateRunning, eventDone)
	require.True(t, ok)
	assert.Equal(t, stateCompleted, to)

	to, ok = table.Apply(stateCompleted, eventStart)
	assert.False(t, ok, "a terminal state has no exit")
	assert.Equal(t, state(""), to)

	to, ok = table.Apply(statePending, eventDone)
	assert.False(t, ok, "an undeclared event does not move the machine")
	assert.Equal(t, state(""), to)
}

func TestValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		table   Table[state, event]
		wantErr string
	}{
		{
			name:  "valid table",
			table: validTable(),
		},
		{
			name:    "no transitions",
			table:   New[state, event]("empty", statePending),
			wantErr: "no transitions",
		},
		{
			name: "duplicate edge",
			table: New("dup", statePending,
				Transition[state, event]{From: statePending, Event: eventStart, To: stateRunning},
				Transition[state, event]{From: statePending, Event: eventStart, To: stateCompleted},
			).Terminal(stateRunning, stateCompleted),
			wantErr: `duplicate edge "pending" on event "start"`,
		},
		{
			name: "state with no exit",
			table: New("dead-end", statePending,
				Transition[state, event]{From: statePending, Event: eventStart, To: stateRunning},
				Transition[state, event]{From: statePending, Event: eventDone, To: stateCompleted},
			).Terminal(stateCompleted),
			wantErr: `state "running" has no exit`,
		},
		{
			name: "terminal state nothing leads to",
			table: New("orphan-terminal", statePending,
				Transition[state, event]{From: statePending, Event: eventStart, To: stateRunning},
			).Terminal(stateRunning, stateCompleted),
			wantErr: `state "completed" is unreachable from "pending"`,
		},
		{
			name: "terminal state with an exit",
			table: New("terminal-exit", statePending,
				Transition[state, event]{From: statePending, Event: eventStart, To: stateRunning},
				Transition[state, event]{From: stateRunning, Event: eventDone, To: statePending},
			).Terminal(stateRunning),
			wantErr: `terminal state "running" has an exit`,
		},
		{
			name: "unreachable state",
			table: New("unreachable", statePending,
				Transition[state, event]{From: statePending, Event: eventStart, To: stateRunning},
				Transition[state, event]{From: stateRunning, Event: eventDone, To: stateCompleted},
				Transition[state, event]{From: stateFailed, Event: eventStart, To: stateRunning},
			).Terminal(stateCompleted),
			wantErr: `state "failed" is unreachable from "pending"`,
		},
		{
			// The first offending state in order, not whichever the map yields.
			name: "several unreachable states name the first",
			table: New("many-unreachable", statePending,
				Transition[state, event]{From: statePending, Event: eventStart, To: stateRunning},
				Transition[state, event]{From: state("zeta"), Event: eventStart, To: stateRunning},
				Transition[state, event]{From: state("alpha"), Event: eventStart, To: stateRunning},
			).Terminal(stateRunning),
			wantErr: `state "alpha" is unreachable from "pending"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.table.Validate()

			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestMustPanicsOnInvalidTable(t *testing.T) {
	t.Parallel()

	assert.PanicsWithValue(t,
		`machine "broken": state "running" has no exit`,
		func() {
			Must(New("broken", statePending,
				Transition[state, event]{From: statePending, Event: eventStart, To: stateRunning},
			))
		})
}

func TestMermaid(t *testing.T) {
	t.Parallel()

	got := validTable().Mermaid()

	assert.Equal(t, `stateDiagram-v2
    state "pending" as s0
    [*] --> s0
    state "running" as s1
    s0 --> s1 : start
    state "completed" as s2
    s1 --> s2 : done
    state "failed" as s3
    s1 --> s3 : fail
    s1 --> s3 : fail-again
    s2 --> [*]
    s3 --> [*]
`, got)
}
