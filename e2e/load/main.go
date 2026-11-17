// Command load seeds, runs and judges the k6 load scenarios in this
// directory against the local deployment; see README.md. k6 generates the
// traffic; this command does what k6 cannot see: the participants, the
// database during the run, and the state the run left behind.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/alecthomas/kong"
)

type cli struct {
	Context   string `default:"k3d-fundament" help:"kubectl context of the local k3d cluster."`
	Namespace string `default:"fundament" help:"Namespace of the deployment."`

	Seed   SeedCmd   `cmd:"" help:"Seed participants for the agents scenario."`
	Run    RunCmd    `cmd:"" help:"Run a scenario and judge it."`
	Report ReportCmd `cmd:"" help:"Judge a finished run again, e.g. with other limits."`
	Volume VolumeCmd `cmd:"" help:"Add processed rows to the authz outbox."`
}

func main() {
	var c cli
	parsed := kong.Parse(&c, kong.Name("load"), kong.UsageOnError())
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	// The first Ctrl-C ends the run and still records its end state; a second
	// one stops the command at once.
	go func() {
		<-ctx.Done()
		stop()
	}()
	k, err := newKube(c.Context, c.Namespace)
	if err == nil {
		parsed.BindTo(ctx, (*context.Context)(nil))
		err = parsed.Run(k)
	}
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "load:", err)
		os.Exit(1)
	}
}
