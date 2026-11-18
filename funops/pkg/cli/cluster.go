package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/fundament-oss/fundament/funops/pkg/db/gen"
)

// ClusterCmd groups the cluster commands.
type ClusterCmd struct {
	Sync ClusterSyncCmd `cmd:"" help:"Queue a sync of a cluster to Gardener, or of every cluster."`
}

// ClusterSyncCmd queues a sync, so the cluster-worker writes the Shoot again
// the way it does after a change to the cluster. A release that changes what
// the worker writes to every Shoot reaches the existing Shoots this way.
type ClusterSyncCmd struct {
	Cluster string `arg:"" optional:"" help:"Cluster ID."`
	All     bool   `help:"Queue a sync of every cluster."`
}

// clusterSyncOutput is the JSON output structure of cluster sync.
type clusterSyncOutput struct {
	Queued []string `json:"queued"`
}

// Run executes the cluster sync command.
func (c *ClusterSyncCmd) Run(ctx *Context) error {
	if c.All == (c.Cluster != "") {
		return errors.New("give a cluster ID or --all, not both")
	}

	var clusterID pgtype.UUID
	if c.Cluster != "" {
		id, err := uuid.Parse(c.Cluster)
		if err != nil {
			return fmt.Errorf("cluster ID %q is not a UUID", c.Cluster)
		}
		clusterID = pgtype.UUID{Bytes: id, Valid: true}
	}

	queued, err := ctx.Queries.ClusterQueueSync(context.Background(), db.ClusterQueueSyncParams{ClusterID: clusterID})
	if err != nil {
		return fmt.Errorf("failed to queue cluster sync: %w", err)
	}

	ids := make([]string, 0, len(queued))
	for i := range queued {
		ids = append(ids, uuid.UUID(queued[i].Bytes).String())
	}

	switch ctx.Output {
	case OutputJSON:
		return PrintJSON(clusterSyncOutput{Queued: ids})
	case OutputTable:
		for _, id := range ids {
			fmt.Println(id)
		}
	default:
		panic(fmt.Sprintf("unhandled output format: %q", ctx.Output))
	}

	ctx.Logger.Info("cluster syncs queued", "count", len(ids))

	return nil
}
