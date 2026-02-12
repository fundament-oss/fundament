package handler

import (
	"context"
	"fmt"
	"log/slog"

	openfga "github.com/openfga/go-sdk"
	"github.com/openfga/go-sdk/client"

	"github.com/fundament-oss/fundament/common/authz"
)

// Handler contains all entity handlers for the authz worker.
type Handler struct {
	fga    *client.OpenFgaClient
	logger *slog.Logger
}

// New creates a new Handlers instance.
func New(fga *client.OpenFgaClient, logger *slog.Logger) *Handler {
	return &Handler{fga: fga, logger: logger}
}

func (h *Handler) writeTuples(ctx context.Context, tuples ...openfga.TupleKey) error {
	if len(tuples) == 0 {
		return nil
	}
	if _, err := h.fga.WriteTuples(ctx).Body(tuples).Execute(); err != nil {
		return fmt.Errorf("write tuples: %w", err)
	}
	return nil
}

func (h *Handler) deleteTuples(ctx context.Context, tuples ...openfga.TupleKeyWithoutCondition) error {
	if len(tuples) == 0 {
		return nil
	}
	if _, err := h.fga.DeleteTuples(ctx).Body(tuples).Execute(); err != nil {
		return fmt.Errorf("delete tuples: %w", err)
	}
	return nil
}

// deleteTuplesIfExist deletes tuples, ignoring errors if the tuples don't exist.
func (h *Handler) deleteTuplesIfExist(ctx context.Context, tuples ...openfga.TupleKeyWithoutCondition) error {
	if len(tuples) == 0 {
		return nil
	}
	opts := client.ClientWriteOptions{
		Conflict: client.ClientWriteConflictOptions{
			OnMissingDeletes: client.CLIENT_WRITE_REQUEST_ON_MISSING_DELETES_IGNORE,
		},
	}
	if _, err := h.fga.DeleteTuples(ctx).Body(tuples).Options(opts).Execute(); err != nil {
		return fmt.Errorf("delete tuples: %w", err)
	}
	return nil
}

func tuple(subject authz.Object, relation authz.ActionName, object authz.Object) openfga.TupleKey {
	return openfga.TupleKey{
		User:     subject.String(),
		Relation: string(relation),
		Object:   object.String(),
	}
}

func tupleDelete(subject authz.Object, relation authz.ActionName, object authz.Object) openfga.TupleKeyWithoutCondition {
	return openfga.TupleKeyWithoutCondition{
		User:     subject.String(),
		Relation: string(relation),
		Object:   object.String(),
	}
}
