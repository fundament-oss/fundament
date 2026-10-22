// Package db holds the staff fleet queries of platform-api (FUN-23). The admin
// role reads every organization, so filtering out deleted rows is the queries'
// job. drift.sql has one query per fleet.v1.DriftKind; each compares only what
// the database holds, tenant.gardener_shoots being cluster-worker's record of
// its last successful ListShoots.
//
// The public overview has no queries here: sqlc cannot infer the columns of
// tenant.platform_overview(), so pkg/overview scans it with pgx.
//
//go:generate sqlc generate
package db
