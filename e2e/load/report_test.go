package main

import (
	"bytes"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cleanEnd() endState { return endState{RunStart: 100, RunEnd: 110, BreakerMs: 5000} }

// samplesEvery samples every 2 s, as the sampler does.
func samplesEvery(start, end int64) []sample {
	var s []sample
	for t := start; t <= end; t += 2 {
		s = append(s, sample{T: t, Conns: 10, MaxConns: 100})
	}
	return s
}

func TestParseEventsSkipsK6Lines(t *testing.T) {
	log := `time="..." level=info msg="k6 started"
{"t":1000,"kind":"fail","name":"login","status":503,"detail":"down"}
{"t":2000,"kind":"ok","name":"login","status":200}
{"not":"an event"}
`
	events, err := parseEvents(strings.NewReader(log))
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, "fail", events[0].Kind)
	assert.Equal(t, "down", events[0].Detail)
}

func TestFailuresPerCallAndStatus(t *testing.T) {
	events := []event{
		{Kind: "fail", Name: "login", Status: 503.0, Detail: "first"},
		{Kind: "fail", Name: "login", Status: 503.0, Detail: "second"},
		{Kind: "fail", Name: "login", Status: 0.0},
		{Kind: "check", Name: "own-clusters", Status: "check"},
		{Kind: "ok", Name: "login", Status: 200.0},
	}
	list := failures(events)
	require.Len(t, list, 3)
	assert.Equal(t, failure{Name: "login", Status: "503", Count: 2, Detail: "first"}, list[0])
}

func TestOutages(t *testing.T) {
	events := []event{
		{T: 1000, Kind: "fail", Name: "login"},
		{T: 2500, Kind: "fail", Name: "login"},
		{T: 4000, Kind: "ok", Name: "login"},
		{T: 5000, Kind: "fail", Name: "login"},
		{T: 5500, Kind: "ok", Name: "login"},
		{T: 1000, Kind: "ok", Name: "projects"},
		{T: 3000, Kind: "fail", Name: "deny"},
		{T: 3000, Kind: "check", Name: "owner-grant"},
	}
	assert.Equal(t, []outage{
		{Name: "login", Seconds: 3, Recovered: true},
		{Name: "deny", Seconds: math.Inf(1)},
	}, outages(events))
}

func TestParseSample(t *testing.T) {
	s, ok := parseSample("1794931677,18,100,3,1.5,0,fun_cluster_worker=4 postgres=1")
	require.True(t, ok)
	assert.Equal(t, sample{T: 1794931677, Conns: 18, MaxConns: 100, Unprocessed: 3, Lag: 1.5, Users: "fun_cluster_worker=4 postgres=1"}, s)

	_, ok = parseSample("ERROR:  canceling statement")
	assert.False(t, ok)
}

func TestLongestGapCountsTheRunsEnds(t *testing.T) {
	s := samplesEvery(104, 106)
	assert.Equal(t, int64(4), longestGap(s, 100, 110))
	assert.Equal(t, int64(10), longestGap(nil, 100, 110))
}

func TestParseEndStateNeedsEveryField(t *testing.T) {
	s, err := parseEndState(strings.NewReader("run_start=1\nrun_end=2\nbreaker_threshold_ms=5000\noutbox_unprocessed=0\noutbox_failed=0\nprojects_left=3\n"))
	require.NoError(t, err)
	assert.Equal(t, endState{RunStart: 1, RunEnd: 2, BreakerMs: 5000, ProjectsLeft: 3}, s)

	_, err = parseEndState(strings.NewReader("run_start=1\nrun_end=x\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "run_end")
	assert.Contains(t, err.Error(), "projects_left")
}

func TestEndStateRoundTrip(t *testing.T) {
	want := endState{RunStart: 1, RunEnd: 2, BreakerMs: 2500, OutboxUnprocessed: 3, OutboxFailed: 4, ProjectsLeft: 5}
	var buf bytes.Buffer
	require.NoError(t, want.write(&buf, true))
	got, err := parseEndState(&buf)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

// An end state the database could not complete keeps the run's own fields,
// and the report counts the rest against the run.
func TestIncompleteEndState(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, endState{RunStart: 100, RunEnd: 110, BreakerMs: 5000, ProjectsLeft: 9}.write(&buf, false))
	got, err := parseEndState(&buf)
	require.Error(t, err)
	assert.Equal(t, endState{RunStart: 100, RunEnd: 110, BreakerMs: 5000}, got)
	for _, f := range []string{"outbox_unprocessed", "outbox_failed", "projects_left"} {
		assert.Contains(t, err.Error(), f)
	}

	var out bytes.Buffer
	breaches := judge(&out, &results{Samples: samplesEvery(100, 110), End: got, EndError: err}, limits{MaxLag: 5, MaxConn: 0.9, MaxGap: 10})
	require.Len(t, breaches, 1)
	assert.Contains(t, breaches[0], "incomplete end state")
}

func TestJudge(t *testing.T) {
	base := limits{MaxLag: 5, MaxConn: 0.9, MaxGap: 10}
	clean := results{Samples: samplesEvery(100, 110), End: cleanEnd()}

	cases := []struct {
		name   string
		mutate func(*results, *limits)
		want   []string
	}{
		{name: "clean run passes", mutate: func(*results, *limits) {}},
		{
			name:   "no samples",
			mutate: func(r *results, _ *limits) { r.Samples = nil },
			want:   []string{"no database samples"},
		},
		{
			name:   "samples stop early",
			mutate: func(r *results, _ *limits) { r.Samples = samplesEvery(100, 100); r.End.RunEnd = 120 },
			want:   []string{"no database sample for 20 s"},
		},
		{
			name:   "connections near the limit",
			mutate: func(r *results, _ *limits) { r.Samples[2].Conns = 95 },
			want:   []string{"database connections peaked at 95 of 100"},
		},
		{
			name:   "outbox lag over the breaker",
			mutate: func(r *results, _ *limits) { r.Samples[1].Lag = 6 },
			want:   []string{"oldest unprocessed authz outbox row reached 6.0 s"},
		},
		{
			name: "dirty end state",
			mutate: func(r *results, _ *limits) {
				r.End.OutboxUnprocessed, r.End.OutboxFailed, r.End.ProjectsLeft = 1, 2, 3
			},
			want: []string{"1 authz outbox rows still unprocessed", "2 authz outbox rows failed", "3 projects of this run"},
		},
		{
			name:   "missing end state",
			mutate: func(r *results, _ *limits) { r.EndError = assert.AnError },
			want:   []string{assert.AnError.Error()},
		},
		{
			name: "outage over the limit",
			mutate: func(r *results, l *limits) {
				l.MaxOutage = new(float64(10))
				r.Events = []event{{T: 0, Kind: "fail", Name: "login"}, {T: 12000, Kind: "ok", Name: "login"}}
			},
			want: []string{"login failed for 12.0 s, limit 10 s"},
		},
		{
			name: "outage that never recovered",
			mutate: func(r *results, l *limits) {
				l.MaxOutage = new(float64(10))
				r.Events = []event{{T: 0, Kind: "ok", Name: "login"}, {T: 1000, Kind: "fail", Name: "login"}}
			},
			want: []string{"login failed for never recovered"},
		},
		{
			name: "outage limit without any success",
			mutate: func(r *results, l *limits) {
				l.MaxOutage = new(float64(10))
				r.Events = nil
			},
			want: []string{"no successful request was logged"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := clean
			r.Samples = append([]sample(nil), clean.Samples...)
			l := base
			tc.mutate(&r, &l)
			var out bytes.Buffer
			breaches := judge(&out, &r, l)
			require.Len(t, breaches, len(tc.want), out.String())
			for i, w := range tc.want {
				assert.Contains(t, breaches[i], w)
			}
		})
	}
}

func TestEnvLimit(t *testing.T) {
	t.Setenv("LOAD_TEST_LIMIT", "")
	v, err := envLimit("LOAD_TEST_LIMIT")
	require.NoError(t, err)
	assert.Nil(t, v)

	t.Setenv("LOAD_TEST_LIMIT", "2.5")
	v, err = envLimit("LOAD_TEST_LIMIT")
	require.NoError(t, err)
	assert.InDelta(t, 2.5, *v, 0)

	for _, bad := range []string{"ten", "-1", "NaN", "Inf"} {
		t.Setenv("LOAD_TEST_LIMIT", bad)
		_, err = envLimit("LOAD_TEST_LIMIT")
		assert.Error(t, err, bad)
	}
}

func TestEnvShare(t *testing.T) {
	t.Setenv("LOAD_TEST_SHARE", "")
	v, err := envShareOr("LOAD_TEST_SHARE", 0.9)
	require.NoError(t, err)
	assert.InDelta(t, 0.9, v, 0)

	t.Setenv("LOAD_TEST_SHARE", "1.5")
	_, err = envShareOr("LOAD_TEST_SHARE", 0.9)
	assert.Error(t, err)
}
