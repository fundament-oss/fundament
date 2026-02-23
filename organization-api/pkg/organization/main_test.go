package organization_test

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

const testDBPort = 45325

func TestMain(m *testing.M) {
	cacheDir := os.Getenv("FUNDAMENT_TEST_CACHE_DIR")
	if cacheDir == "" {
		userCache, err := os.UserCacheDir()
		if err != nil {
			log.Fatalf("failed to determine user cache directory: %v", err)
		}

		cacheDir = filepath.Join(userCache, "fundament-test-pg")
	}

	err := os.MkdirAll(cacheDir, 0o755)
	if err != nil {
		log.Fatalf("failed to create cache directory: %v", err)
	}

	dataDir := filepath.Join(cacheDir, "data")
	pgBin := filepath.Join(cacheDir, "bin")

	var stopPostgres func()

	if dirExists(dataDir) && dirExists(pgBin) {
		log.Printf("existing embedded-postgres detected at %q", cacheDir)
		startExistingEmbeddedPostgres(pgBin, dataDir)

		stopPostgres = func() {
			pgCtl := filepath.Join(pgBin, "pg_ctl")
			cmd := exec.Command(pgCtl, "-D", dataDir, "-w", "-m", "fast", "stop")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				log.Printf("failed to stop postgres: %v", err)
			}
		}
	} else {
		log.Printf("setting up new embedded-postgres installation %q", cacheDir)
		epDB := createAndStartNewEmbeddedPostgres(cacheDir, dataDir)

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

	adminPool := newAdminPool()
	defer adminPool.Close()

	createRoles(adminPool)

	err = setupTemplateDatabaseWithMigrations(adminPool)
	if err != nil {
		log.Fatalf("failed to setup template database: %v", err)
	}

	code := m.Run()

	stopPostgres()
	os.Exit(code)
}

func startExistingEmbeddedPostgres(pgBin, dataDir string) {
	// We run `pg_ctl` directly, since the `Start` method of embedded-postgres deletes
	// the postgres binaries every time. There is no workaround currently.
	// See https://github.com/fergusstrange/embedded-postgres/issues/154
	removeStalePostmasterPID(pgBin, dataDir)

	pgCtl := filepath.Join(pgBin, "pg_ctl")
	cmd := exec.Command(pgCtl,
		"-D", dataDir,
		"-w",
		"-o", fmt.Sprintf("-p %d -c fsync=off -c synchronous_commit=off", testDBPort),
		"start",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("failed to start postgres from cache: %v", err)
	}
}

// removeStalePostmasterPID handles a leftover postmaster.pid from a previous
// crashed test run. If the referenced process is no longer running it removes
// the file so pg_ctl can start fresh. If the process is still alive it stops
// it gracefully via pg_ctl stop -w before returning.
func removeStalePostmasterPID(pgBin, dataDir string) {
	pidFile := filepath.Join(dataDir, "postmaster.pid")

	data, err := os.ReadFile(pidFile)
	if err != nil {
		return // no pid file, nothing to clean up
	}

	// The first line of postmaster.pid contains the PID.
	lines := strings.SplitN(string(data), "\n", 2)
	if len(lines) == 0 {
		return
	}

	pid, err := strconv.Atoi(strings.TrimSpace(lines[0]))
	if err != nil {
		return
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		log.Printf("removing stale postmaster.pid (pid %d)", pid)
		os.Remove(pidFile)
		return
	}

	// On Unix, FindProcess always succeeds. Send signal 0 to check if alive.
	if err := process.Signal(syscall.Signal(0)); err != nil {
		log.Printf("removing stale postmaster.pid (pid %d no longer running)", pid)
		os.Remove(pidFile)
		return
	}

	// Process is still running — stop it gracefully before we start a fresh instance.
	log.Printf("stopping already-running postgres (pid %d) before restart", pid)
	pgCtl := filepath.Join(pgBin, "pg_ctl")
	cmd := exec.Command(pgCtl, "-D", dataDir, "-w", "-m", "fast", "stop")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("failed to stop existing postgres (pid %d): %v", pid, err)
	}
}

func createAndStartNewEmbeddedPostgres(runtimePath, dataDir string) *embeddedpostgres.EmbeddedPostgres {
	err := os.RemoveAll(dataDir)
	if err != nil {
		log.Fatalf("failed to remove old data directory: %v", err)
	}

	epDB := embeddedpostgres.NewDatabase(
		embeddedpostgres.DefaultConfig().
			Version(embeddedpostgres.V18).
			Port(testDBPort).
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

func setupTemplateDatabaseWithMigrations(pool *pgxpool.Pool) error {
	projectRoot := findProjectRoot()

	_, err := pool.Exec(context.Background(), "UPDATE pg_database SET datistemplate = false WHERE datname = 'fundament'")
	if err != nil {
		return fmt.Errorf("failed to unmark fundament as template: %v", err)
	}

	trekApply(projectRoot)

	_, err = pool.Exec(context.Background(), "UPDATE pg_database SET datistemplate = true WHERE datname = 'fundament'")
	if err != nil {
		return fmt.Errorf("failed to mark fundament as template: %v", err)
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
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func newAdminPool() *pgxpool.Pool {
	pool, err := pgxpool.New(context.Background(), fmt.Sprintf("postgres://postgres:postgres@localhost:%d/postgres?sslmode=disable", testDBPort))
	if err != nil {
		log.Fatalf("failed to connect to postgres: %v", err)
	}
	return pool
}

func createRoles(pool *pgxpool.Pool) {
	ctx := context.Background()
	roles := []string{"fun_authn_api", "fun_fundament_api", "fun_operator", "fun_owner", "fun_authz", "fun_cluster_worker", "fun_authz_worker"}
	for _, role := range roles {
		_, err := pool.Exec(ctx, fmt.Sprintf(`DO $$ BEGIN CREATE ROLE %s WITH LOGIN; EXCEPTION WHEN duplicate_object THEN NULL; END $$`, role))
		if err != nil {
			log.Fatalf("failed to create role %s: %v", role, err)
		}
	}
}

func trekApply(projectRoot string) {
	cmd := exec.Command("trek", "apply",
		"--reset-database",
		"--insert-test-data",
		"--postgres-host", "localhost",
		"--postgres-port", fmt.Sprintf("%d", testDBPort),
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
