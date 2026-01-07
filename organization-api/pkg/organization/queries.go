package organization

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
)

var (
	// ErrNoTenantInContext is returned when tenant_id is not found in context.
	ErrNoTenantInContext = errors.New("tenant_id not found in context")

	// ErrConnectionAcquire is returned when database connection acquisition fails.
	ErrConnectionAcquire = errors.New("failed to acquire database connection")

	// ErrSetTenantContext is returned when setting tenant context in PostgreSQL fails.
	ErrSetTenantContext = errors.New("failed to set tenant context")
)

// QueryProvider manages context-aware database queries with Row Level Security.
// It acquires connections from the pool and automatically sets tenant context
// for PostgreSQL RLS enforcement.
type QueryProvider struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// NewQueryProvider creates a new QueryProvider instance.
func NewQueryProvider(pool *pgxpool.Pool, logger *slog.Logger) *QueryProvider {
	return &QueryProvider{
		pool:   pool,
		logger: logger,
	}
}

// obtainQueries returns a *db.Queries with PostgreSQL tenant context set.
// The connection is acquired from the pool and must be released by calling
// the returned release function.
//
// Usage:
//
//	queries, release, err := qp.obtainQueries(ctx)
//	if err != nil {
//	    return err
//	}
//	defer release()
//
//	result, err := queries.SomeQuery(ctx, ...)
func (qp *QueryProvider) obtainQueries(ctx context.Context) (*db.Queries, func(), error) {
	// Extract tenant_id from context
	tenantID, ok := TenantIDFromContext(ctx)
	if !ok {
		return nil, nil, ErrNoTenantInContext
	}

	// Acquire connection from pool
	conn, err := qp.pool.Acquire(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrConnectionAcquire, err)
	}

	// Create release function
	release := func() {
		conn.Release()
	}

	// Set tenant context
	queries := db.New(conn)
	if err := queries.SetTenantContext(ctx, tenantID.String()); err != nil {
		release()
		return nil, nil, fmt.Errorf("%w: %v", ErrSetTenantContext, err)
	}

	return queries, release, nil
}

// WithTransaction begins a transaction with RLS context set and returns the queries and transaction.
// The caller is responsible for committing or rolling back the transaction.
//
// Usage:
//
//	queries, tx, err := qp.WithTransaction(ctx)
//	if err != nil {
//	    return err
//	}
//	defer tx.Rollback(ctx)
//
//	if err := queries.Operation1(ctx, ...); err != nil {
//	    return err
//	}
//	if err := queries.Operation2(ctx, ...); err != nil {
//	    return err
//	}
//
//	if err := tx.Commit(ctx); err != nil {
//	    return err
//	}
func (qp *QueryProvider) WithTransaction(ctx context.Context) (*db.Queries, pgx.Tx, error) {
	tenantID, ok := TenantIDFromContext(ctx)
	if !ok {
		return nil, nil, ErrNoTenantInContext
	}

	// Begin transaction
	tx, err := qp.pool.Begin(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("begin transaction: %w", err)
	}

	// Set tenant context on transaction
	queries := db.New(tx)
	if err := queries.SetTenantContext(ctx, tenantID.String()); err != nil {
		tx.Rollback(ctx)
		return nil, nil, fmt.Errorf("set tenant context: %w", err)
	}

	return queries, tx, nil
}
