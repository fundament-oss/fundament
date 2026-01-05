package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	"github.com/fundament-oss/fundament/organization-api/pkg/models"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *OrganizationServer) GetTenant(
	ctx context.Context,
	req *connect.Request[organizationv1.GetTenantRequest],
) (*connect.Response[organizationv1.GetTenantResponse], error) {
	claims, err := s.validateRequest(req.Header())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	tenantID, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, fmt.Errorf("cluster id parse: %w", err)
	}

	input := models.TenantGet{ID: tenantID}
	if err := s.validator.Validate(input); err != nil {
		return nil, err
	}

	if claims.TenantID != input.ID {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("access denied to tenant"))
	}

	tenant, err := s.queries.TenantGetByID(ctx, input.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("tenant not found"))
		}
		s.logger.Error("failed to get tenant", "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get tenant: %w", err))
	}

	return connect.NewResponse(&organizationv1.GetTenantResponse{
		Tenant: dbTenantToProto(tenant),
	}), nil
}

func (s *OrganizationServer) UpdateTenant(
	ctx context.Context,
	req *connect.Request[organizationv1.UpdateTenantRequest],
) (*connect.Response[organizationv1.UpdateTenantResponse], error) {
	claims, err := s.validateRequest(req.Header())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	tenantID, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, fmt.Errorf("cluster id parse: %w", err)
	}

	input := models.TenantUpdate{ID: tenantID, Name: req.Msg.Name}
	if err := s.validator.Validate(input); err != nil {
		return nil, err
	}

	if claims.TenantID != input.ID {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("access denied to tenant"))
	}

	tenant, err := s.queries.TenantUpdate(ctx, db.TenantUpdateParams{
		ID:   input.ID,
		Name: input.Name,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("tenant not found"))
		}
		s.logger.Error("failed to update tenant", "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update tenant: %w", err))
	}

	s.logger.Info("tenant updated", "tenant_id", tenant.ID, "name", tenant.Name)

	return connect.NewResponse(&organizationv1.UpdateTenantResponse{
		Tenant: dbTenantToProto(tenant),
	}), nil
}

func dbTenantToProto(t db.OrganizationTenant) *organizationv1.Tenant {
	return &organizationv1.Tenant{
		Id:      t.ID.String(),
		Name:    t.Name,
		Created: t.Created.Time.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// WithTxTenant will create a transaction, attach the current tenant id and execute the fn.
// Everything in fn will be set ontop of the same transaction.
// When it is done, it will commit the transaction and exit.
func WithTxTenant(ctx context.Context, pool *pgxpool.Pool, tenantID uuid.UUID, fn func(*db.Queries) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}

	defer tx.Rollback(ctx)

	queries := db.New(tx.Conn())

	if err := queries.SetTenantContext(ctx, tenantID.String()); err != nil {
		return fmt.Errorf("set tenant context: %w", err)
	}

	if err := fn(queries); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return nil
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
func WithTenant(pool *pgxpool.Pool, tenantID uuid.UUID) *TenantDB {
	return &TenantDB{
		pool:     pool,
		tenantID: tenantID,
	}
}

// Exec implements the DBTX interface.
func (t *TenantDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	var result pgconn.CommandTag
	err := t.pool.AcquireFunc(ctx, func(conn *pgxpool.Conn) error {
		queries := db.New(conn)
		if err := queries.SetTenantContext(ctx, t.tenantID.String()); err != nil {
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

	queries := db.New(conn)
	if err := queries.SetTenantContext(ctx, t.tenantID.String()); err != nil {
		return nil, fmt.Errorf("setting tenant context: %w", err)
	}

	queryrows, err := conn.Query(ctx, sql, args...)
	if err != nil {
		conn.Release()
		return nil, err
	}

	// Wrap rows to release connection when closed
	return &rows{Rows: queryrows, conn: conn}, nil
}

// QueryRow implements the DBTX interface.
func (t *TenantDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	// QueryRow needs to return a Row that will release the connection after Scan.
	conn, err := t.pool.Acquire(ctx)
	if err != nil {
		return &errorRow{err: fmt.Errorf("acquiring connection: %w", err)}
	}

	queries := db.New(conn)
	if err := queries.SetTenantContext(ctx, t.tenantID.String()); err != nil {
		return &errorRow{err: fmt.Errorf("set tenant context: %w", err)}
	}

	queryrow := conn.QueryRow(ctx, sql, args...)
	return &row{Row: queryrow, conn: conn}
}

// rows wraps pgx.Rows to release the connection when closed.
type rows struct {
	pgx.Rows
	conn *pgxpool.Conn
}

func (r *rows) Close() {
	r.Rows.Close()
	r.conn.Release()
}

// row wraps pgx.Row to release the connection after Scan.
type row struct {
	pgx.Row
	conn *pgxpool.Conn
}

func (r *row) Scan(dest ...any) error {
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
