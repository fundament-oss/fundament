// Package db holds the staff fleet queries of platform-api (FUN-23). The admin
// role reads every organization, so filtering out deleted rows is the queries'
// job, and so is leaving out the system organization
// (tenant.system_organization_id()). drift.sql reads the findings
// cluster-worker's reconcile loop records; nothing is compared here.
//
// The public overview has no queries here: sqlc cannot infer the columns of
// tenant.platform_overview(), so pkg/overview scans it with pgx.
//
//go:generate sqlc generate
package db
