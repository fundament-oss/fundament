package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
)

const unprocessedQuery = "select count(*) from authz.outbox where processed is null and status in ('pending', 'retrying')"

// sampleQuery is one sampler row but its time: client connections to the
// instance (OpenFGA's database too), max_connections, unprocessed outbox rows,
// the oldest one's age in seconds (what organization-api's breaker reads),
// failed rows, and connections per database user.
const sampleQuery = "select (select count(*) from pg_stat_activity where backend_type = 'client backend')" +
	", current_setting('max_connections')" +
	", (" + unprocessedQuery + ")" +
	", (select coalesce(extract(epoch from now() - min(created)), 0)::numeric(10, 1) from authz.outbox where processed is null and status in ('pending', 'retrying'))" +
	", (select count(*) from authz.outbox where status = 'failed')" +
	", (select string_agg(usename || '=' || count, ' ' order by count desc) from (select usename, count(*) from pg_stat_activity where backend_type = 'client backend' group by usename))"

const sampleEvery = 2 * time.Second

// RunCmd runs one k6 scenario, samples the database meanwhile, then checks the end state.
type RunCmd struct {
	Scenario string   `arg:"" enum:"agents,browse,probe" help:"Scenario: agents, browse or probe."`
	Args     []string `arg:"" optional:"" help:"Arguments for k6, after --, e.g. -- -e AGENTS=300."`
	Out      string   `default:"reports/load" help:"Directory for the participants and the results." type:"path"`
	Scripts  string   `default:"load" help:"Directory of the k6 scripts." type:"path"`
}

func (c *RunCmd) Run(ctx context.Context, k kube) error {
	breaker, err := breakerThreshold(ctx, k)
	if err != nil {
		return err
	}
	l, err := limitsFor(breaker)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(c.Out, 0o750); err != nil {
		return fmt.Errorf("creating %s: %w", c.Out, err)
	}
	for _, f := range []string{eventsFile(c.Out, c.Scenario), samplesFile(c.Out, c.Scenario), endFile(c.Out, c.Scenario)} {
		if err := os.Remove(f); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("removing %s: %w", f, err)
		}
	}
	edge, err := output(ctx, "docker", "inspect", "-f", fmt.Sprintf(`{{(index .NetworkSettings.Networks "k3d-%s").IPAddress}}`, k.cluster()), "k3d-"+k.cluster()+"-server-0")
	if err != nil {
		return err
	}
	samples, err := os.Create(samplesFile(c.Out, c.Scenario))
	if err != nil {
		return fmt.Errorf("creating samples file: %w", err)
	}

	run := strconv.FormatInt(time.Now().Unix(), 16)
	start := time.Now().Unix()
	sampling, stop := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Go(func() { k.sample(sampling, samples, "load-sampler-"+run) })

	k6Err := c.k6(ctx, k, edge, run)
	end := time.Now().Unix()
	stop()
	wg.Wait()
	if err := samples.Close(); err != nil {
		return fmt.Errorf("writing samples: %w", err)
	}

	// The end state is recorded after Ctrl-C too. The time it may take covers
	// a Postgres restart, which waits up to 180 s for open sessions to end
	// before it stops.
	settling, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
	defer cancel()
	state, settleErr := settle(settling, k, run, endState{RunStart: start, RunEnd: end, BreakerMs: breaker.Milliseconds()})
	if settleErr != nil {
		fmt.Fprintf(os.Stderr, "end state: %v\n", settleErr)
	}
	if err := writeEndState(endFile(c.Out, c.Scenario), state, settleErr == nil); err != nil {
		return err
	}

	r, err := loadResults(c.Out, c.Scenario)
	if err != nil {
		return err
	}
	var failed error
	if breaches := judge(os.Stdout, &r, l); len(breaches) > 0 {
		failed = fmt.Errorf("%d breaches", len(breaches))
	}
	if err := errors.Join(k6Err, settleErr, failed); err != nil {
		return fmt.Errorf("load %s failed: %w", c.Scenario, err)
	}
	return nil
}

// writeEndState writes the end state; an incomplete one holds only what the
// run knew itself, and the report counts the fields it lacks.
func writeEndState(path string, s endState, complete bool) error {
	f, err := os.Create(path) //nolint:gosec // a file of the run's own output directory
	if err != nil {
		return fmt.Errorf("creating end state file: %w", err)
	}
	if err := s.write(f, complete); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("writing end state: %w", err)
	}
	return nil
}

// k6 runs the scenario in docker on the cluster's network. EDGE sends every
// request to the k3d node's port 443: the k3d serverlb in front of
// 127.0.0.1:8443 runs nginx with 1024 file descriptors and drops connections
// from a few hundred clients.
//
// On Ctrl-C docker passes the interrupt on, and k6 stops its scenarios and
// writes its summary; a container that outlives that is removed.
func (c *RunCmd) k6(ctx context.Context, k kube, edge, run string) error {
	uid := strconv.Itoa(os.Getuid())
	name := "load-" + c.Scenario + "-" + run
	args := slices.Concat([]string{
		"run", "--rm", "--name", name, "--network", "k3d-" + k.cluster(), "-u", uid,
		"-e", "EDGE=" + edge, "-e", "INSECURE=1", "-e", "RUN=" + run, "-e", "MAX_OUTAGE=" + os.Getenv("MAX_OUTAGE"),
		"-e", "PEOPLE=/reports/" + participantsName,
		"-v", c.Scripts + ":/scripts:ro", "-v", c.Out + ":/reports", "grafana/k6:1.4.2",
		"run", "--log-format", "raw", "--log-output", "file=/reports/" + c.Scenario + "-events.jsonl",
		"--summary-export", "/reports/" + c.Scenario + ".summary.json",
	}, c.Args, []string{"/scripts/" + c.Scenario + ".js"})
	cmd := exec.CommandContext(ctx, "docker", args...) //nolint:gosec // k6 arguments from the user's own command line
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 30 * time.Second
	err := cmd.Run()
	if ctx.Err() != nil {
		_, _ = output(context.WithoutCancel(ctx), "docker", "rm", "-f", name)
	}
	if err != nil {
		return fmt.Errorf("k6: %w", err)
	}
	return nil
}

// breakerThreshold is organization-api's CIRCUIT_BREAKER_THRESHOLD: beyond
// it the breaker refuses every request.
func breakerThreshold(ctx context.Context, k kube) (time.Duration, error) {
	raw, err := k.env(ctx, "organization-api", "CIRCUIT_BREAKER_THRESHOLD")
	if err != nil {
		return 0, err
	}
	if raw == "" {
		// organization-api's own default when the deployment does not set it.
		return 5 * time.Second, nil
	}
	threshold, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("organization-api CIRCUIT_BREAKER_THRESHOLD=%s: %w", raw, err)
	}
	return threshold, nil
}

// limitsFor reads the limits from the environment. MAX_LAG defaults to the
// circuit breaker's threshold.
func limitsFor(breaker time.Duration) (limits, error) {
	maxOutage, err := envLimit("MAX_OUTAGE")
	if err != nil {
		return limits{}, err
	}
	l := limits{MaxOutage: maxOutage}
	var errs [3]error
	l.MaxLag, errs[0] = envLimitOr("MAX_LAG", breaker.Seconds())
	l.MaxConn, errs[1] = envLimitOr("MAX_CONN", 0.9)
	l.MaxGap, errs[2] = envLimitOr("MAX_GAP", 10)
	if err := errors.Join(errs[:]...); err != nil {
		return l, fmt.Errorf("limits: %w", err)
	}
	return l, nil
}

// settle waits up to a minute for authz-worker to finish what the run queued,
// then reads the rest of the end state into s. A query that fails, such as
// during a failover, is tried again until ctx ends.
func settle(ctx context.Context, k kube, run string, s endState) (endState, error) {
	var pod string
	count := func(query string) (int64, error) {
		for {
			var out string
			var err error
			if pod == "" {
				pod, err = k.primary(ctx)
			}
			if err == nil {
				if out, err = output(ctx, "kubectl", k.psql(pod, "load", "-c", query)...); err == nil {
					n, err := strconv.ParseInt(out, 10, 64)
					if err != nil {
						return 0, fmt.Errorf("%s: %w", query, err)
					}
					return n, nil
				}
				// After a failover the primary is another pod.
				pod = ""
			}
			select {
			case <-ctx.Done():
				return 0, fmt.Errorf("%w; last error: %w", ctx.Err(), err)
			case <-time.After(2 * time.Second):
			}
		}
	}
	var err error
	for i := range 30 {
		if s.OutboxUnprocessed, err = count(unprocessedQuery); err != nil {
			return s, err
		}
		if s.OutboxUnprocessed == 0 || i == 29 {
			break
		}
		select {
		case <-ctx.Done():
			return s, fmt.Errorf("waiting for authz-worker: %w", ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
	if s.OutboxFailed, err = count(fmt.Sprintf("select count(*) from authz.outbox where status = 'failed' and created >= to_timestamp(%d)", s.RunStart)); err != nil {
		return s, err
	}
	s.ProjectsLeft, err = count(fmt.Sprintf("select count(*) from tenant.projects where deleted is null and (name like 'l%[1]s-%%' or name like 'p%[1]s-%%')", run))
	return s, err
}

// sample writes a sampler row every sampleEvery until ctx ends. A lost
// session is opened again; report counts the gap it leaves.
func (k kube) sample(ctx context.Context, out io.Writer, app string) {
	for ctx.Err() == nil {
		if err := k.watch(ctx, out, app); err != nil && ctx.Err() == nil {
			fmt.Fprintf(os.Stderr, "database sampler: %v; reconnecting\n", err)
		}
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
		}
	}
}

// watch keeps one psql session on the primary that runs sampleQuery every
// sampleEvery, so sampling costs one connection for the whole run and goes
// on while Postgres has no connection left to give. It returns when the
// session ends or stays silent for three intervals.
func (k kube) watch(ctx context.Context, out io.Writer, app string) error {
	pod, err := k.primary(ctx)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, "kubectl", k.psql(pod, app)...) //nolint:gosec // the primary's pod name, from kubectl
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("psql stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("psql stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting psql: %w", err)
	}
	lines := make(chan string)
	go func() {
		defer close(lines)
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			select {
			case lines <- sc.Text():
			case <-ctx.Done():
				return
			}
		}
	}()
	// Wait closes stdout, so it comes once the reader is done with it. psql
	// in the pod outlives kubectl and would go on watching, so its session
	// is ended by name.
	defer func() {
		cancel()
		for range lines {
		}
		_ = cmd.Wait()
		stopping, done := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer done()
		if _, err := output(stopping, "kubectl", k.psql(pod, "load", "-c", fmt.Sprintf("select count(pg_terminate_backend(pid)) from pg_stat_activity where application_name = '%s'", app))...); err != nil {
			fmt.Fprintf(os.Stderr, "database sampler: ending session %s: %v\n", app, err)
		}
	}()
	// stdin stays open: psql ends at its end.
	if _, err := fmt.Fprintf(stdin, "%s \\watch %g\n", sampleQuery, sampleEvery.Seconds()); err != nil {
		return fmt.Errorf("writing to psql: %w", err)
	}

	for {
		select {
		case row, ok := <-lines:
			if !ok {
				return errors.New("psql session ended")
			}
			// Stamped with this machine's clock, as the run's start and end
			// are: the cluster's clock may differ, as in a VM after sleep.
			line := strconv.FormatInt(time.Now().Unix(), 10) + "," + row
			if _, ok := parseSample(line); ok {
				if _, err := fmt.Fprintln(out, line); err != nil {
					return fmt.Errorf("writing sample: %w", err)
				}
			}
		case <-time.After(3 * sampleEvery):
			return errors.New("no sample for three intervals")
		case <-ctx.Done():
			return nil
		}
	}
}

// ReportCmd judges a run again from the files it left, e.g. with other limits.
type ReportCmd struct {
	Scenario string `arg:"" enum:"agents,browse,probe" help:"Scenario: agents, browse or probe."`
	Out      string `default:"reports/load" help:"Directory of the results." type:"path"`
}

func (c *ReportCmd) Run() error {
	r, err := loadResults(c.Out, c.Scenario)
	if err != nil {
		return err
	}
	if r.End.BreakerMs <= 0 && os.Getenv("MAX_LAG") == "" {
		return errors.New("the run's end state holds no circuit breaker threshold: set MAX_LAG")
	}
	l, err := limitsFor(time.Duration(r.End.BreakerMs) * time.Millisecond)
	if err != nil {
		return err
	}
	if breaches := judge(os.Stdout, &r, l); len(breaches) > 0 {
		return fmt.Errorf("%d breaches", len(breaches))
	}
	return nil
}

// VolumeCmd adds processed rows to the authz outbox: the history of an
// installation that has run for a while.
type VolumeCmd struct {
	Rows int `arg:"" optional:"" default:"2000000" help:"Rows to add."`
}

func (c *VolumeCmd) Run(ctx context.Context, k kube) error {
	if c.Rows <= 0 {
		return fmt.Errorf("rows must be positive, not %d", c.Rows)
	}
	// A row names what it is about; outbox_ck_single_fk wants exactly one.
	project, err := k.sql(ctx, "select id from tenant.projects where deleted is null order by created limit 1")
	if err != nil {
		return err
	}
	id, err := uuid.Parse(project)
	if err != nil {
		return errors.New("no project for the rows to name: seed first (just e2e load-seed) or create one")
	}
	_, err = k.sql(ctx, fmt.Sprintf("insert into authz.outbox (project_id, created, processed, status) "+
		"select '%[1]s', now() - interval '30 days' + generate_series * interval '1 second', now() - interval '30 days' + generate_series * interval '1 second', 'completed' "+
		"from generate_series(1, %[2]d)", id, c.Rows))
	if err == nil {
		fmt.Printf("added %d processed rows to the authz outbox\n", c.Rows)
	}
	return err
}
