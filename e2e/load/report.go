package main

import (
	"bufio"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// limits are what a run may reach beyond k6's own thresholds.
type limits struct {
	// MaxOutage, in seconds, is how long a call may keep failing; nil leaves
	// refused requests to k6's thresholds.
	MaxOutage *float64
	// MaxLag, in seconds, is how old the oldest unprocessed authz outbox row may get.
	MaxLag float64
	// MaxConn is the share of max_connections the database may reach.
	MaxConn float64
	// MaxGap, in seconds, is the longest stretch without a database sample.
	MaxGap float64
}

// envLimit reads a limit from the environment. A value that is not a number
// would make every comparison false and pass, so it is an error.
func envLimit(name string) (*float64, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value < 0 || math.IsInf(value, 0) || math.IsNaN(value) {
		return nil, fmt.Errorf("%s=%s is not a non-negative number", name, raw)
	}
	return &value, nil
}

func envLimitOr(name string, fallback float64) (float64, error) {
	value, err := envLimit(name)
	if err != nil || value == nil {
		return fallback, err
	}
	return *value, nil
}

// envShareOr reads a share, from 0 to 1.
func envShareOr(name string, fallback float64) (float64, error) {
	value, err := envLimitOr(name, fallback)
	if err == nil && value > 1 {
		err = fmt.Errorf("%s=%g is a share: at most 1", name, value)
	}
	return value, err
}

// event is a line lib.js logs: a fail, an ok or a failed check.
type event struct {
	T      int64  `json:"t"`
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Status any    `json:"status"`
	Detail string `json:"detail"`
}

// parseEvents reads the k6 log, which also holds k6's own lines; only the
// JSON events count.
func parseEvents(r io.Reader) ([]event, error) {
	var events []event
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var e event
		if json.Unmarshal(sc.Bytes(), &e) == nil && e.Kind != "" {
			events = append(events, e)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("reading events: %w", err)
	}
	return events, nil
}

type failure struct {
	Name   string
	Status string
	Count  int
	Detail string
}

// failures counts the failed requests and checks per call and status, with
// the first example of each, the most frequent first.
func failures(events []event) []failure {
	type key struct{ name, status string }
	index := map[key]int{}
	var list []failure
	for _, e := range events {
		if e.Kind == "ok" {
			continue
		}
		k := key{e.Name, fmt.Sprint(e.Status)}
		i, ok := index[k]
		if !ok {
			i = len(list)
			index[k] = i
			list = append(list, failure{Name: k.name, Status: k.status, Detail: e.Detail})
		}
		list[i].Count++
	}
	slices.SortStableFunc(list, func(a, b failure) int { return b.Count - a.Count })
	return list
}

// outage is the longest stretch one call kept failing.
type outage struct {
	Name      string
	Seconds   float64
	Recovered bool
}

// outages finds, per call, the longest time from a failure to the next
// success of that call by any VU. A scenario that calls heartbeat() logs
// successes once a second per VU, so the bound is about a second coarse.
// Calls that never failed are left out.
func outages(events []event) []outage {
	byName := map[string][]event{}
	var names []string
	for _, e := range events {
		if e.Kind != "fail" && e.Kind != "ok" {
			continue
		}
		if _, ok := byName[e.Name]; !ok {
			names = append(names, e.Name)
		}
		byName[e.Name] = append(byName[e.Name], e)
	}
	var result []outage
	for _, name := range names {
		list := byName[name]
		slices.SortStableFunc(list, func(a, b event) int { return cmp.Compare(a.T, b.T) })
		var worst int64
		var since *int64
		for _, e := range list {
			switch {
			case e.Kind == "fail" && since == nil:
				since = &e.T
			case e.Kind == "ok" && since != nil:
				worst = max(worst, e.T-*since)
				since = nil
			}
		}
		switch {
		case since != nil:
			result = append(result, outage{Name: name, Seconds: math.Inf(1)})
		case worst > 0:
			result = append(result, outage{Name: name, Seconds: float64(worst) / 1000, Recovered: true})
		}
	}
	return result
}

// sample is one row of the database sampler.
type sample struct {
	T           int64
	Conns       float64
	MaxConns    float64
	Unprocessed float64
	Lag         float64
	Failed      float64
	Users       string
}

// parseSample reads a sampler row: the time, client connections,
// max_connections, unprocessed outbox rows, the oldest one's age, failed rows
// and connections per database user.
func parseSample(line string) (sample, bool) {
	f := strings.SplitN(line, ",", 7)
	if len(f) < 6 {
		return sample{}, false
	}
	t, err := strconv.ParseInt(f[0], 10, 64)
	if err != nil {
		return sample{}, false
	}
	nums := make([]float64, 5)
	for i := range nums {
		if nums[i], err = strconv.ParseFloat(f[i+1], 64); err != nil {
			return sample{}, false
		}
	}
	s := sample{T: t, Conns: nums[0], MaxConns: nums[1], Unprocessed: nums[2], Lag: nums[3], Failed: nums[4]}
	if len(f) == 7 {
		s.Users = f[6]
	}
	return s, true
}

func parseSamples(r io.Reader) ([]sample, error) {
	var samples []sample
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		if s, ok := parseSample(sc.Text()); ok {
			samples = append(samples, s)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("reading samples: %w", err)
	}
	return samples, nil
}

// longestGap is the longest stretch without a sample between start and end,
// so samples that start late or stop early count too.
func longestGap(samples []sample, start, end int64) int64 {
	times := make([]int64, 0, len(samples)+2)
	times = append(times, start)
	for _, s := range samples {
		times = append(times, s.T)
	}
	times = append(times, end)
	slices.Sort(times)
	var gap int64
	for i := 1; i < len(times); i++ {
		gap = max(gap, times[i]-times[i-1])
	}
	return gap
}

// endState is when the run ran, organization-api's circuit breaker threshold
// at the time, and what the run left behind once authz-worker caught up.
type endState struct {
	RunStart          int64
	RunEnd            int64
	BreakerMs         int64
	OutboxUnprocessed int64
	OutboxFailed      int64
	ProjectsLeft      int64
}

var endStateFields = []string{"run_start", "run_end", "breaker_threshold_ms", "outbox_unprocessed", "outbox_failed", "projects_left"}

// runFields are the fields the run knows without asking the database.
const runFields = 3

func (s *endState) fields() []*int64 {
	return []*int64{&s.RunStart, &s.RunEnd, &s.BreakerMs, &s.OutboxUnprocessed, &s.OutboxFailed, &s.ProjectsLeft}
}

// write writes the fields as key=value lines; without complete, only the
// run's own.
func (s endState) write(w io.Writer, complete bool) error {
	fields := s.fields()
	if !complete {
		fields = fields[:runFields]
	}
	for i, v := range fields {
		if _, err := fmt.Fprintf(w, "%s=%d\n", endStateFields[i], *v); err != nil {
			return fmt.Errorf("writing end state: %w", err)
		}
	}
	return nil
}

// parseEndState reads key=value lines; every field must be there and a number.
func parseEndState(r io.Reader) (endState, error) {
	values := map[string]string{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		if k, v, ok := strings.Cut(sc.Text(), "="); ok {
			values[k] = v
		}
	}
	if err := sc.Err(); err != nil {
		return endState{}, fmt.Errorf("reading end state: %w", err)
	}
	var s endState
	targets := s.fields()
	var errs []error
	for i, name := range endStateFields {
		v, err := strconv.ParseInt(values[name], 10, 64)
		if err != nil {
			errs = append(errs, fmt.Errorf("end state %s is missing or not a number", name))
			continue
		}
		*targets[i] = v
	}
	if len(errs) > 0 {
		return s, fmt.Errorf("incomplete end state: %w", errors.Join(errs...))
	}
	return s, nil
}

// results are what one run collected.
type results struct {
	Events   []event
	Samples  []sample
	End      endState
	EndError error
}

// judge prints the report and returns every limit the run crossed.
func judge(w io.Writer, r *results, l limits) []string {
	var breaches []string
	p := func(format string, args ...any) { _, _ = fmt.Fprintf(w, format+"\n", args...) }

	p("\n== failures by call and status")
	list := failures(r.Events)
	if len(list) == 0 {
		p("none")
	}
	for _, f := range list {
		detail := strings.Join(strings.Fields(f.Detail), " ")
		p("%8d  %s  %s  %s", f.Count, f.Name, f.Status, detail[:min(len(detail), 100)])
	}

	if l.MaxOutage != nil {
		if !slices.ContainsFunc(r.Events, func(e event) bool { return e.Kind == "ok" }) {
			breaches = append(breaches, "no successful request was logged, so no outage can be measured")
		}
		p("\n== longest outage per call (limit %g s)", *l.MaxOutage)
		for _, o := range outages(r.Events) {
			label := "never recovered"
			if o.Recovered {
				label = fmt.Sprintf("%.1f s", o.Seconds)
			}
			p("%-14s %s", o.Name, label)
			if o.Seconds > *l.MaxOutage {
				breaches = append(breaches, fmt.Sprintf("%s failed for %s, limit %g s", o.Name, label, *l.MaxOutage))
			}
		}
	}

	if len(r.Samples) == 0 {
		breaches = append(breaches, "no database samples")
	} else {
		if r.End.RunStart > 0 && r.End.RunEnd > 0 {
			// A gap means the sampler lost its session, most likely because
			// Postgres or its pod was gone.
			if gap := longestGap(r.Samples, r.End.RunStart, r.End.RunEnd); float64(gap) > l.MaxGap {
				breaches = append(breaches, fmt.Sprintf("no database sample for %d s, limit %g s", gap, l.MaxGap))
			}
		}
		peak := slices.MaxFunc(r.Samples, func(a, b sample) int { return cmp.Compare(a.Conns, b.Conns) })
		var maxConns, unprocessed, lag float64
		for _, s := range r.Samples {
			maxConns = max(maxConns, s.MaxConns)
			unprocessed = max(unprocessed, s.Unprocessed)
			lag = max(lag, s.Lag)
		}
		p("\n== database (%d samples)", len(r.Samples))
		p("connections   peak %g of %g (limit %g %%)", peak.Conns, maxConns, l.MaxConn*100)
		p("outbox        peak %g unprocessed, oldest %.1f s (limit %g s)", unprocessed, lag, l.MaxLag)
		p("at the peak   %s", peak.Users)
		if peak.Conns > maxConns*l.MaxConn {
			breaches = append(breaches, fmt.Sprintf("database connections peaked at %g of %g", peak.Conns, maxConns))
		}
		if lag > l.MaxLag {
			breaches = append(breaches, fmt.Sprintf("oldest unprocessed authz outbox row reached %.1f s, limit %g s", lag, l.MaxLag))
		}
	}

	p("\n== end state")
	if r.EndError != nil {
		breaches = append(breaches, r.EndError.Error())
	} else {
		for i, v := range r.End.fields() {
			p("%-28s %d", endStateFields[i], *v)
		}
		if r.End.OutboxUnprocessed > 0 {
			breaches = append(breaches, fmt.Sprintf("%d authz outbox rows still unprocessed", r.End.OutboxUnprocessed))
		}
		if r.End.OutboxFailed > 0 {
			breaches = append(breaches, fmt.Sprintf("%d authz outbox rows failed during the run", r.End.OutboxFailed))
		}
		if r.End.ProjectsLeft > 0 {
			breaches = append(breaches, fmt.Sprintf("%d projects of this run were not deleted", r.End.ProjectsLeft))
		}
	}

	if len(breaches) > 0 {
		p("\n== report: FAILED")
		for _, b := range breaches {
			p("- %s", b)
		}
	} else {
		p("\n== report: passed")
	}
	return breaches
}

// Files of one run in the output directory.
func eventsFile(dir, scenario string) string  { return filepath.Join(dir, scenario+"-events.jsonl") }
func samplesFile(dir, scenario string) string { return filepath.Join(dir, scenario+"-db.csv") }
func endFile(dir, scenario string) string     { return filepath.Join(dir, scenario+"-verify.txt") }

// loadResults reads what a run left in dir. A missing file reads as empty,
// which the report counts against the run.
func loadResults(dir, scenario string) (results, error) {
	var r results
	read := func(path string, parse func(io.Reader) error) error {
		f, err := os.Open(path) //nolint:gosec // a file of the run's own output directory
		if errors.Is(err, os.ErrNotExist) {
			return parse(strings.NewReader(""))
		}
		if err != nil {
			return fmt.Errorf("opening %s: %w", path, err)
		}
		defer func() { _ = f.Close() }()
		return parse(f)
	}
	err := errors.Join(
		read(eventsFile(dir, scenario), func(f io.Reader) (err error) { r.Events, err = parseEvents(f); return err }),
		read(samplesFile(dir, scenario), func(f io.Reader) (err error) { r.Samples, err = parseSamples(f); return err }),
		read(endFile(dir, scenario), func(f io.Reader) error { r.End, r.EndError = parseEndState(f); return nil }),
	)
	if err != nil {
		return r, fmt.Errorf("reading the run's files: %w", err)
	}
	return r, nil
}
