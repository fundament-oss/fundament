package gardener

// MsgShootAwaitingHealth is the status message cluster-worker stores for a
// freshly created, ready cluster whose health checks have not passed yet
// (status_rules.go, settlingAfterCreate). organization-api keys the STARTING
// cluster status on it, so writer and reader must share one definition —
// rewording a mirrored literal would silently turn every settling cluster
// into an "Unhealthy" one.
const MsgShootAwaitingHealth = "Cluster is ready, waiting for health checks to pass"
