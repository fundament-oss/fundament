// Package testpg runs the embedded Postgres that platform-api's database
// tests share. Each test package calls Main from its TestMain with its own
// port and data directory: go test runs packages at once, and a second
// postmaster over one PGDATA stops the first.
package testpg

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/testdb"
)

// Main starts (or reuses) the instance, builds the fundament template database
// from the migrations, runs the tests and exits.
func Main(m *testing.M, port int, instanceDir string) {
	// The environment variable is the cache root CI restores, not the data
	// directory itself, so packages can share the cache entry.
	cacheRoot := os.Getenv("FUNDAMENT_TEST_CACHE_DIR")
	if cacheRoot == "" {
		userCache, err := os.UserCacheDir()
		if err != nil {
			log.Fatalf("failed to determine user cache directory: %v", err)
		}
		cacheRoot = userCache
	}

	cacheDir := filepath.Join(cacheRoot, instanceDir)
	if err := os.MkdirAll(cacheDir, 0o750); err != nil { //nolint:gosec // test helper, paths are not user-controlled
		log.Fatalf("failed to create cache directory: %v", err)
	}

	dataDir := filepath.Join(cacheDir, "data")
	pgBin := filepath.Join(cacheDir, "bin")

	var stopPostgres func()
	if dirExists(dataDir) && dirExists(pgBin) {
		log.Printf("existing embedded-postgres detected at %q", cacheDir) //nolint:gosec // test log
		startExisting(pgBin, dataDir, port)
		stopPostgres = func() {
			cmd := exec.Command(filepath.Join(pgBin, "pg_ctl"), "-D", dataDir, "-w", "-m", "fast", "stop") //nolint:gosec,noctx // test helper
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				log.Printf("failed to stop postgres: %v", err)
			}
		}
	} else {
		log.Printf("setting up new embedded-postgres installation at %q", cacheDir) //nolint:gosec // test log
		epDB := createAndStart(cacheDir, dataDir, port)
		stopPostgres = func() {
			if err := epDB.Stop(); err != nil {
				log.Printf("failed to stop postgres: %v", err)
			}
		}
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		log.Println("received signal, stopping postgres...")
		stopPostgres()
		os.Exit(1)
	}()

	// Closed explicitly: log.Fatalf and os.Exit below both skip defers.
	adminPool, err := pgxpool.New(context.Background(), superDSN(port, "postgres"))
	if err != nil {
		log.Fatalf("failed to connect to postgres: %v", err)
	}
	testdb.UseGlobalTrustAuth(dataDir, adminPool)
	testdb.CreateRoles(context.Background(), adminPool)
	err = setupTemplate(adminPool, port)
	adminPool.Close()
	if err != nil {
		stopPostgres()
		log.Fatalf("failed to setup template database: %v", err)
	}

	code := m.Run()

	stopPostgres()
	os.Exit(code)
}

// Env is one test's database, cloned from the template. Super connects as
// postgres and seeds around RLS; Role connects as the role under test.
type Env struct {
	Name  string
	Port  int
	Super *pgxpool.Pool
}

// NewEnv clones a database named after the test.
func NewEnv(t *testing.T, port int) *Env {
	t.Helper()

	name := dbName(t.Name())

	adminPool, err := pgxpool.New(context.Background(), superDSN(port, "postgres"))
	require.NoError(t, err)
	defer adminPool.Close()

	_, err = adminPool.Exec(context.Background(), fmt.Sprintf(`DROP DATABASE IF EXISTS %q WITH (FORCE)`, name))
	require.NoError(t, err)
	_, err = adminPool.Exec(context.Background(), fmt.Sprintf(`CREATE DATABASE %q TEMPLATE fundament`, name))
	require.NoError(t, err)

	super, err := pgxpool.New(context.Background(), superDSN(port, name))
	require.NoError(t, err)
	t.Cleanup(super.Close)

	return &Env{Name: name, Port: port, Super: super}
}

// RoleDSN connects to this test's database as role, through its grants and
// policies rather than around them.
func (e *Env) RoleDSN(role string) string {
	return fmt.Sprintf("postgres://%s@localhost:%d/%s?sslmode=disable", role, e.Port, e.Name)
}

// RolePool opens a pool as role, closed when the test ends.
func (e *Env) RolePool(t *testing.T, role string) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), e.RoleDSN(role))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

func superDSN(port int, database string) string {
	return fmt.Sprintf("postgres://postgres:postgres@localhost:%d/%s?sslmode=disable", port, database)
}

func dbName(testName string) string {
	name := strings.ToLower(testName)
	name = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(name, "_")
	name = strings.Trim(name, "_")
	if len(name) > 63 {
		name = name[:63]
	}
	return name
}

func startExisting(pgBin, dataDir string, port int) {
	// pg_ctl directly: embedded-postgres's Start deletes the binaries every
	// time. See https://github.com/fergusstrange/embedded-postgres/issues/154
	removeStalePostmasterPID(pgBin, dataDir)

	cmd := exec.Command(filepath.Join(pgBin, "pg_ctl"), //nolint:gosec,noctx // test helper
		"-D", dataDir,
		"-w",
		"-o", fmt.Sprintf("-p %d -c fsync=off -c synchronous_commit=off", port),
		"start",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("failed to start postgres from cache: %v", err)
	}
}

func removeStalePostmasterPID(pgBin, dataDir string) {
	pidFile := filepath.Join(dataDir, "postmaster.pid")
	data, err := os.ReadFile(pidFile) //nolint:gosec // test helper
	if err != nil {
		return
	}
	lines := strings.SplitN(string(data), "\n", 2)
	pid, err := strconv.Atoi(strings.TrimSpace(lines[0]))
	if err != nil {
		return
	}
	process, err := os.FindProcess(pid)
	if err != nil || process.Signal(syscall.Signal(0)) != nil {
		log.Printf("removing stale postmaster.pid (pid %d)", pid)
		_ = os.Remove(pidFile) //nolint:gosec // test helper
		return
	}

	log.Printf("stopping already-running postgres (pid %d) before restart", pid)
	cmd := exec.Command(filepath.Join(pgBin, "pg_ctl"), "-D", dataDir, "-w", "-m", "fast", "stop") //nolint:gosec,noctx // test helper
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("failed to stop existing postgres (pid %d): %v", pid, err)
	}
}

func createAndStart(runtimePath, dataDir string, port int) *embeddedpostgres.EmbeddedPostgres {
	if err := os.RemoveAll(dataDir); err != nil { //nolint:gosec // test helper
		log.Fatalf("failed to remove old data directory: %v", err)
	}
	epDB := embeddedpostgres.NewDatabase(
		embeddedpostgres.DefaultConfig().
			Version(embeddedpostgres.V18).
			Port(uint32(port)). //nolint:gosec // a fixed test port
			Username("postgres").
			Password("postgres").
			Database("postgres").
			RuntimePath(runtimePath).
			StartParameters(map[string]string{
				"fsync":              "off",
				"synchronous_commit": "off",
			}),
	)
	if err := epDB.Start(); err != nil {
		log.Fatalf("failed to start embedded postgres: %v", err)
	}
	return epDB
}

func setupTemplate(pool *pgxpool.Pool, port int) error {
	projectRoot := findProjectRoot()

	if _, err := pool.Exec(context.Background(), "UPDATE pg_database SET datistemplate = false WHERE datname = 'fundament'"); err != nil {
		return fmt.Errorf("failed to unmark fundament as template: %w", err)
	}

	trekApply(projectRoot, port)

	// trek loads db/migrations and db/testdata only. The system organization
	// lives in db/seed (applied in real environments by the db-migrations
	// Job); both platform views leave it out, so the tests need it present.
	if err := applySeed(projectRoot, port, "0100-system-org.sql"); err != nil {
		return err
	}

	if _, err := pool.Exec(context.Background(), "UPDATE pg_database SET datistemplate = true WHERE datname = 'fundament'"); err != nil {
		return fmt.Errorf("failed to mark fundament as template: %w", err)
	}
	return nil
}

func applySeed(projectRoot string, port int, name string) error {
	conn, err := pgx.Connect(context.Background(), superDSN(port, "fundament"))
	if err != nil {
		return fmt.Errorf("failed to connect to fundament db for seed: %w", err)
	}
	defer func() { _ = conn.Close(context.Background()) }()

	seedSQL, err := os.ReadFile(filepath.Join(projectRoot, "db", "seed", name)) //nolint:gosec // test helper
	if err != nil {
		return fmt.Errorf("failed to read seed %s: %w", name, err)
	}
	// Simple protocol: the whole multi-statement file in one implicit transaction.
	if _, err := conn.PgConn().Exec(context.Background(), string(seedSQL)).ReadAll(); err != nil {
		return fmt.Errorf("failed to apply seed %s: %w", name, err)
	}
	return nil
}

func findProjectRoot() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		log.Fatal("failed to get caller information")
	}
	dir := filepath.Dir(filename)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			log.Fatal("could not find project root (go.mod)")
		}
		dir = parent
	}
}

func dirExists(path string) bool {
	info, err := os.Stat(path) //nolint:gosec // test helper
	return err == nil && info.IsDir()
}

func trekApply(projectRoot string, port int) {
	trekBin, err := exec.LookPath("trek")
	if err != nil {
		for _, c := range []string{
			filepath.Join(os.Getenv("HOME"), ".local", "share", "mise", "shims", "trek"),
			filepath.Join(os.Getenv("HOME"), ".asdf", "shims", "trek"),
		} {
			if _, statErr := os.Stat(c); statErr == nil { //nolint:gosec // test helper
				trekBin = c
				break
			}
		}
		if trekBin == "" {
			log.Fatalf("trek binary not found in PATH or common tool-manager locations: %v", err)
		}
	}

	cmd := exec.Command(trekBin, "apply", //nolint:gosec,noctx // test helper
		"--reset-database",
		"--insert-test-data",
		"--postgres-host", "localhost",
		"--postgres-port", strconv.Itoa(port),
		"--postgres-user", "postgres",
		"--postgres-password", "postgres",
	)
	cmd.Dir = filepath.Join(projectRoot, "db")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("trek apply failed: %v", err)
	}
}
