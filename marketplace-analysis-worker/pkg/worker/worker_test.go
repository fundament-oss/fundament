package worker_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/marketplace-analysis-worker/pkg/analyze"
	"github.com/fundament-oss/fundament/marketplace-analysis-worker/pkg/worker"
)

type testEnv struct {
	superPool  *pgxpool.Pool
	workerPool *pgxpool.Pool
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	name := strings.Trim(regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(strings.ToLower(t.Name()), "_"), "_")
	if len(name) > 63 {
		name = name[:63]
	}

	adminPool, err := pgxpool.New(context.Background(),
		fmt.Sprintf("postgres://postgres:postgres@localhost:%d/postgres?sslmode=disable", testDBPort))
	require.NoError(t, err)
	defer adminPool.Close()
	_, err = adminPool.Exec(context.Background(), fmt.Sprintf(`DROP DATABASE IF EXISTS %q WITH (FORCE)`, name))
	require.NoError(t, err)
	_, err = adminPool.Exec(context.Background(), fmt.Sprintf(`CREATE DATABASE %q TEMPLATE fundament`, name))
	require.NoError(t, err)

	superPool, err := pgxpool.New(context.Background(),
		fmt.Sprintf("postgres://postgres:postgres@localhost:%d/%s?sslmode=disable", testDBPort, name))
	require.NoError(t, err)
	t.Cleanup(superPool.Close)

	// The worker connects as its own role, so its grants and policies are
	// what the tests exercise.
	workerPool, err := pgxpool.New(context.Background(),
		fmt.Sprintf("postgres://fun_marketplace_analysis_worker@localhost:%d/%s?sslmode=disable", testDBPort, name))
	require.NoError(t, err)
	t.Cleanup(workerPool.Close)

	return &testEnv{superPool: superPool, workerPool: workerPool}
}

func (env *testEnv) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	_, err := env.superPool.Exec(context.Background(), sql, args...)
	require.NoError(t, err)
}

// seedVersion creates a listing with one version in the given status.
func (env *testEnv) seedVersion(t *testing.T, status string) uuid.UUID {
	t.Helper()

	orgID, pluginID, versionID := uuid.New(), uuid.New(), uuid.New()
	env.exec(t, `INSERT INTO tenant.organizations (id, name, alias) VALUES ($1, $2, $2)`, orgID, "org-"+orgID.String()[:8])
	env.exec(t, `INSERT INTO appstore.plugins (id, organization_id, name, display_name, description, visibility)
		VALUES ($1, $2, 'cert-manager', 'cert-manager', '', 'public')`, pluginID, orgID)
	env.exec(t, `INSERT INTO appstore.plugin_definitions (id, plugin_id, plugin_version, manifest, hash, image, status)
		VALUES ($1, $2, '1.0.0', 'x', 'sha256:test', 'ghcr.io/example/img@sha256:abc', $3)`, versionID, pluginID, status)
	return versionID
}

func (env *testEnv) submit(t *testing.T, versionID uuid.UUID) {
	t.Helper()

	userID := uuid.New()
	env.exec(t, `INSERT INTO tenant.users (id, name) VALUES ($1, 'dev')`, userID)
	env.exec(t, `UPDATE appstore.submissions SET closed = now() WHERE plugin_definition_id = $1 AND closed IS NULL`, versionID)
	env.exec(t, `INSERT INTO appstore.submissions (plugin_definition_id, submitter_user_id) VALUES ($1, $2)`, versionID, userID)
}

type analysisRow struct {
	status         string
	statusInfo     *string
	retries        int32
	imageDigest    string
	packages       string
	files          string
	filesTruncated bool
	platforms      string
	sbom           []byte
}

func (env *testEnv) analyses(t *testing.T, versionID uuid.UUID) []analysisRow {
	t.Helper()

	rows, err := env.superPool.Query(context.Background(), `
		SELECT status, status_info, retries, image_digest, packages::text, files::text,
			files_truncated, platforms::text, sbom
		FROM appstore.plugin_image_analyses WHERE plugin_definition_id = $1`, versionID)
	require.NoError(t, err)
	defer rows.Close()

	var result []analysisRow
	for rows.Next() {
		var row analysisRow
		require.NoError(t, rows.Scan(&row.status, &row.statusInfo, &row.retries, &row.imageDigest, &row.packages,
			&row.files, &row.filesTruncated, &row.platforms, &row.sbom))
		result = append(result, row)
	}
	require.NoError(t, rows.Err())
	return result
}

type fakeAnalyzer struct {
	result *analyze.Result
	err    error
	images []string
}

func (f *fakeAnalyzer) Analyze(_ context.Context, image string) (*analyze.Result, error) {
	f.images = append(f.images, image)
	return f.result, f.err
}

func newWorker(env *testEnv, analyzer worker.Analyzer) *worker.Worker {
	return worker.New(env.workerPool, analyzer, slog.Default(), worker.Config{MaxRetries: 2})
}

func TestSubmissionEnqueuesOneAnalysis(t *testing.T) {
	env := newTestEnv(t)
	versionID := env.seedVersion(t, "pending")

	env.submit(t, versionID)
	env.submit(t, versionID)

	rows := env.analyses(t, versionID)
	require.Len(t, rows, 1)
	assert.Equal(t, "pending", rows[0].status)
}

func TestResubmissionRetriesFailedAnalysis(t *testing.T) {
	env := newTestEnv(t)
	versionID := env.seedVersion(t, "pending")
	env.submit(t, versionID)
	env.exec(t, `UPDATE appstore.plugin_image_analyses SET status = 'failed', status_info = 'private', retries = 2`)

	env.submit(t, versionID)

	rows := env.analyses(t, versionID)
	require.Len(t, rows, 1)
	assert.Equal(t, "pending", rows[0].status)
	assert.Nil(t, rows[0].statusInfo)
	assert.Equal(t, int32(0), rows[0].retries)
}

func TestSweepEnqueuesReviewableVersionsOnce(t *testing.T) {
	env := newTestEnv(t)
	approved := env.seedVersion(t, "approved")
	draft := env.seedVersion(t, "draft")
	w := newWorker(env, &fakeAnalyzer{})

	enqueued, err := w.Sweep(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(1), enqueued)

	enqueued, err = w.Sweep(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(0), enqueued)

	assert.Len(t, env.analyses(t, approved), 1)
	assert.Empty(t, env.analyses(t, draft))
}

func TestProcessNextRecordsResult(t *testing.T) {
	env := newTestEnv(t)
	versionID := env.seedVersion(t, "pending")
	env.submit(t, versionID)
	analyzer := &fakeAnalyzer{result: &analyze.Result{
		ImageDigest:    "sha256:def",
		Platform:       "linux/amd64",
		Platforms:      []string{"linux/amd64", "linux/arm64"},
		Packages:       []analyze.Package{{Purl: "pkg:deb/debian/openssl@3.0.13", Name: "openssl", Version: "3.0.13", Type: "deb", Licenses: []string{}}},
		Files:          []analyze.File{{Path: "/etc/config", Size: 3, Mode: 0o644, SHA256: "abc"}},
		FilesTruncated: true,
		SBOM:           []byte(`{"bomFormat":"CycloneDX"}`),
	}}

	found, err := newWorker(env, analyzer).ProcessNext(context.Background())
	require.NoError(t, err)
	require.True(t, found)

	assert.Equal(t, []string{"ghcr.io/example/img@sha256:abc"}, analyzer.images)
	rows := env.analyses(t, versionID)
	require.Len(t, rows, 1)
	assert.Equal(t, "completed", rows[0].status)
	assert.JSONEq(t, `[{"purl":"pkg:deb/debian/openssl@3.0.13","name":"openssl","version":"3.0.13","type":"deb","licenses":[]}]`, rows[0].packages)
	assert.JSONEq(t, `["linux/amd64","linux/arm64"]`, rows[0].platforms)
	assert.Equal(t, "sha256:def", rows[0].imageDigest)
	assert.JSONEq(t, `[{"path":"/etc/config","size":3,"mode":420,"sha256":"abc"}]`, rows[0].files)
	assert.True(t, rows[0].filesTruncated)
	assert.JSONEq(t, `{"bomFormat":"CycloneDX"}`, string(rows[0].sbom))

	found, err = newWorker(env, analyzer).ProcessNext(context.Background())
	require.NoError(t, err)
	assert.False(t, found)
}

func TestPermanentFailureIsRecordedForTheReviewer(t *testing.T) {
	env := newTestEnv(t)
	versionID := env.seedVersion(t, "pending")
	env.submit(t, versionID)
	analyzer := &fakeAnalyzer{err: &analyze.PermanentError{
		Reason:    "The image has no linux/amd64 variant, only linux/arm64",
		Platforms: []string{"linux/arm64"},
		Err:       errors.New("detail the reviewer does not need"),
	}}

	_, err := newWorker(env, analyzer).ProcessNext(context.Background())
	require.NoError(t, err)

	rows := env.analyses(t, versionID)
	require.Len(t, rows, 1)
	assert.Equal(t, "failed", rows[0].status)
	require.NotNil(t, rows[0].statusInfo)
	assert.Equal(t, "The image has no linux/amd64 variant, only linux/arm64", *rows[0].statusInfo)
	assert.JSONEq(t, `["linux/arm64"]`, rows[0].platforms)
}

func TestTransientFailureIsRetriedThenGivenUp(t *testing.T) {
	env := newTestEnv(t)
	versionID := env.seedVersion(t, "pending")
	env.submit(t, versionID)
	analyzer := &fakeAnalyzer{err: errors.New("connection reset")}
	w := newWorker(env, analyzer)

	_, err := w.ProcessNext(context.Background())
	require.NoError(t, err)
	rows := env.analyses(t, versionID)
	assert.Equal(t, "retrying", rows[0].status)
	assert.Equal(t, int32(1), rows[0].retries)
	require.NotNil(t, rows[0].statusInfo)
	assert.Equal(t, "Attempt 1 failed: connection reset", *rows[0].statusInfo)

	// Not claimable until its backoff passes.
	found, err := w.ProcessNext(context.Background())
	require.NoError(t, err)
	assert.False(t, found)

	env.exec(t, `UPDATE appstore.plugin_image_analyses SET retry_after = now() - interval '1 second', retries = 2`)
	_, err = w.ProcessNext(context.Background())
	require.NoError(t, err)
	rows = env.analyses(t, versionID)
	assert.Equal(t, "failed", rows[0].status)
}

// A row left running by a worker that died is claimed again once its lease
// passes, and the lost attempt counts.
func TestExpiredLeaseIsClaimedAsAnAttempt(t *testing.T) {
	env := newTestEnv(t)
	versionID := env.seedVersion(t, "pending")
	env.submit(t, versionID)
	env.exec(t, `UPDATE appstore.plugin_image_analyses SET status = 'running', retry_after = now() + interval '1 hour'`)
	analyzer := &fakeAnalyzer{err: errors.New("still broken")}
	w := newWorker(env, analyzer)

	found, err := w.ProcessNext(context.Background())
	require.NoError(t, err)
	assert.False(t, found, "a live lease is not claimable")

	env.exec(t, `UPDATE appstore.plugin_image_analyses SET retry_after = now() - interval '1 second'`)
	found, err = w.ProcessNext(context.Background())
	require.NoError(t, err)
	require.True(t, found)

	rows := env.analyses(t, versionID)
	// One for the lost run, one for this failed attempt.
	assert.Equal(t, int32(2), rows[0].retries)
}

// analyzerFunc lets a test act while an analysis is in flight.
type analyzerFunc func(ctx context.Context, image string) (*analyze.Result, error)

func (f analyzerFunc) Analyze(ctx context.Context, image string) (*analyze.Result, error) {
	return f(ctx, image)
}

func resultWithPackage(name string) *analyze.Result {
	return &analyze.Result{
		ImageDigest: "sha256:def",
		Packages:    []analyze.Package{{Purl: "pkg:generic/" + name, Name: name, Licenses: []string{}}},
	}
}

func TestStaleAttemptCannotOverwriteTheNextClaim(t *testing.T) {
	env := newTestEnv(t)
	versionID := env.seedVersion(t, "pending")
	env.submit(t, versionID)

	// The first attempt outlives its lease: meanwhile a second worker claims
	// the row and records its result. The first one finishes after that.
	stale := analyzerFunc(func(ctx context.Context, _ string) (*analyze.Result, error) {
		env.exec(t, `UPDATE appstore.plugin_image_analyses SET retry_after = now() - interval '1 second'`)
		found, err := newWorker(env, &fakeAnalyzer{result: resultWithPackage("second")}).ProcessNext(ctx)
		require.NoError(t, err)
		require.True(t, found)
		return resultWithPackage("first"), nil
	})

	found, err := newWorker(env, stale).ProcessNext(context.Background())
	require.NoError(t, err)
	require.True(t, found)

	rows := env.analyses(t, versionID)
	require.Len(t, rows, 1)
	assert.Equal(t, "completed", rows[0].status)
	assert.JSONEq(t, `[{"purl":"pkg:generic/second","name":"second","version":"","type":"","licenses":[]}]`, rows[0].packages)
}

func TestClaimSkipsDeletedVersions(t *testing.T) {
	env := newTestEnv(t)
	deleted := env.seedVersion(t, "pending")
	env.submit(t, deleted)
	live := env.seedVersion(t, "pending")
	env.submit(t, live)
	env.exec(t, `UPDATE appstore.plugin_definitions SET deleted = now() WHERE id = $1`, deleted)
	w := newWorker(env, &fakeAnalyzer{result: resultWithPackage("live")})

	found, err := w.ProcessNext(context.Background())
	require.NoError(t, err)
	require.True(t, found)
	found, err = w.ProcessNext(context.Background())
	require.NoError(t, err)
	assert.False(t, found, "the deleted version is never claimed")

	assert.Equal(t, "completed", env.analyses(t, live)[0].status)
	assert.Equal(t, "pending", env.analyses(t, deleted)[0].status)
}

func TestWorkerCannotMoveAnAnalysisToAnotherVersion(t *testing.T) {
	env := newTestEnv(t)
	versionID := env.seedVersion(t, "pending")
	env.submit(t, versionID)
	other := env.seedVersion(t, "pending")

	_, err := env.workerPool.Exec(context.Background(),
		`UPDATE appstore.plugin_image_analyses SET plugin_definition_id = $1`, other)
	require.ErrorContains(t, err, "permission denied")

	_, err = env.workerPool.Exec(context.Background(),
		`UPDATE appstore.plugin_image_analyses SET deleted = now()`)
	require.ErrorContains(t, err, "permission denied")
}

func TestRetryBackoffSurvivesManyAttempts(t *testing.T) {
	env := newTestEnv(t)
	versionID := env.seedVersion(t, "pending")
	env.submit(t, versionID)
	// 1 << 31 overflows int4; the exponent is capped before it gets there.
	env.exec(t, `UPDATE appstore.plugin_image_analyses SET status = 'retrying', retries = 31`)
	w := worker.New(env.workerPool, &fakeAnalyzer{err: errors.New("still broken")}, slog.Default(),
		worker.Config{MaxRetries: 100, MaxBackoff: time.Hour})

	found, err := w.ProcessNext(context.Background())
	require.NoError(t, err)
	require.True(t, found)

	var waiting bool
	require.NoError(t, env.superPool.QueryRow(context.Background(),
		`SELECT retry_after > now() + interval '59 minutes' FROM appstore.plugin_image_analyses`).Scan(&waiting))
	assert.True(t, waiting, "the backoff is capped at max_backoff, not negative")
}
