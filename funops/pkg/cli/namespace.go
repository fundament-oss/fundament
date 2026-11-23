package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/fundament-oss/fundament/funops/pkg/db/gen"
)

// NamespaceCmd groups the namespace commands.
type NamespaceCmd struct {
	Sync NamespaceSyncCmd `cmd:"" help:"Queue a reconcile of a namespace, or of every namespace whose last sync failed."`
}

// NamespaceSyncCmd queues a reconcile, so the cluster-worker converges the
// namespace on its shoot again. The periodic reconcile leaves a failed
// namespace alone until its failure is an hour old; this queues it now, for an
// operator who has fixed what it failed on.
type NamespaceSyncCmd struct {
	Namespace string `arg:"" optional:"" help:"Namespace ID."`
	All       bool   `help:"Queue a reconcile of every namespace whose last sync failed."`
}

// namespaceSyncOutput is the JSON output structure of namespace sync.
type namespaceSyncOutput struct {
	Queued []string `json:"queued"`
}

// Run executes the namespace sync command.
func (c *NamespaceSyncCmd) Run(ctx *Context) error {
	if c.All && c.Namespace != "" {
		return errors.New("give a namespace ID or --all, not both")
	}
	if !c.All && c.Namespace == "" {
		return errors.New("give a namespace ID or --all")
	}

	var namespaceID pgtype.UUID
	if c.Namespace != "" {
		id, err := uuid.Parse(c.Namespace)
		if err != nil {
			return fmt.Errorf("namespace ID %q is not a UUID", c.Namespace)
		}
		namespaceID = pgtype.UUID{Bytes: id, Valid: true}
	}

	queued, err := ctx.Queries.NamespaceQueueSync(context.Background(), db.NamespaceQueueSyncParams{NamespaceID: namespaceID})
	if err != nil {
		return fmt.Errorf("failed to queue namespace sync: %w", err)
	}

	// The query cannot tell these cases apart, and none of them queued a sync.
	if c.Namespace != "" && len(queued) == 0 {
		return fmt.Errorf("no sync queued for namespace %s: it does not exist or already has a sync waiting", c.Namespace)
	}

	ids := make([]string, 0, len(queued))
	for i := range queued {
		ids = append(ids, uuid.UUID(queued[i].Bytes).String())
	}

	switch ctx.Output {
	case OutputJSON:
		return PrintJSON(namespaceSyncOutput{Queued: ids})
	case OutputTable:
		for _, id := range ids {
			fmt.Println(id)
		}
	default:
		panic(fmt.Sprintf("unhandled output format: %q", ctx.Output))
	}

	ctx.Logger.Info("namespace syncs queued", "count", len(ids))

	return nil
}
