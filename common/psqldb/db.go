package psqldb

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
	Pool   *pgxpool.Pool
	logger *slog.Logger
}

func New(ctx context.Context, logger *slog.Logger, databaseURL string) (*DB, error) {
	logger.Debug("creating database connection pool")

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		logger.Error("failed to create connection pool", "error", err)
		return nil, fmt.Errorf("creating connection pool: %w", err)
	}

	logger.Debug("pinging database")
	if err := pool.Ping(ctx); err != nil {
		logger.Error("failed to ping database", "error", err)
		pool.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	logger.Info("database connection established")
	return &DB{
		Pool:   pool,
		logger: logger,
	}, nil
}

func (s *DB) Close() {
	s.logger.Debug("closing database connection pool")
	s.Pool.Close()
}

// TenantDB implements the DBTX interface with automatic tenant context.
// Each query operation acquires a connection, sets the tenant context, executes, and releases.
// This is safe for use with connection pooling and works seamlessly with sqlc.
type TenantDB struct {
	pool     *pgxpool.Pool
	tenantID uuid.UUID
}

// ForTenant returns a DBTX that automatically sets the tenant context for RLS.
// Use this with sqlc's db.New() to get tenant-scoped queries.
func (s *DB) ForTenant(tenantID uuid.UUID) *TenantDB {
	return &TenantDB{
		pool:     s.Pool,
		tenantID: tenantID,
	}
}

// Exec implements the DBTX interface.
func (t *TenantDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	var result pgconn.CommandTag
	err := t.pool.AcquireFunc(ctx, func(conn *pgxpool.Conn) error {
		if _, err := conn.Exec(ctx, "SELECT set_config('app.current_tenant_id', $1, false)", t.tenantID.String()); err != nil {
			return fmt.Errorf("setting tenant context: %w", err)
		}
		var err error
		result, err = conn.Exec(ctx, sql, args...)
		return err
	})
	return result, err
}

// Query implements the DBTX interface.
func (t *TenantDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	// For Query, we need to keep the connection open until rows are closed.
	// We use a different approach: acquire, set context, return rows with connection attached.
	conn, err := t.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquiring connection: %w", err)
	}

	if _, err := conn.Exec(ctx, "SELECT set_config('app.current_tenant_id', $1, false)", t.tenantID.String()); err != nil {
		conn.Release()
		return nil, fmt.Errorf("setting tenant context: %w", err)
	}

	rows, err := conn.Query(ctx, sql, args...)
	if err != nil {
		conn.Release()
		return nil, err
	}

	// Wrap rows to release connection when closed
	return &tenantRows{Rows: rows, conn: conn}, nil
}

// QueryRow implements the DBTX interface.
func (t *TenantDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	// QueryRow needs to return a Row that will release the connection after Scan.
	conn, err := t.pool.Acquire(ctx)
	if err != nil {
		return &errorRow{err: fmt.Errorf("acquiring connection: %w", err)}
	}

	if _, err := conn.Exec(ctx, "SELECT set_config('app.current_tenant_id', $1, false)", t.tenantID.String()); err != nil {
		conn.Release()
		return &errorRow{err: fmt.Errorf("setting tenant context: %w", err)}
	}

	row := conn.QueryRow(ctx, sql, args...)
	return &tenantRow{Row: row, conn: conn}
}

// tenantRows wraps pgx.Rows to release the connection when closed.
type tenantRows struct {
	pgx.Rows
	conn *pgxpool.Conn
}

func (r *tenantRows) Close() {
	r.Rows.Close()
	r.conn.Release()
}

// tenantRow wraps pgx.Row to release the connection after Scan.
type tenantRow struct {
	pgx.Row
	conn *pgxpool.Conn
}

func (r *tenantRow) Scan(dest ...any) error {
	defer r.conn.Release()
	return r.Row.Scan(dest...)
}

// errorRow is a pgx.Row that returns an error on Scan.
type errorRow struct {
	err error
}

func (r *errorRow) Scan(dest ...any) error {
	return r.err
}
