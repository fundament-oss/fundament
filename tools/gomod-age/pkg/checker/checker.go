// Package checker implements the core dependency age checking logic.
package checker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/fundament-oss/fundament/tools/gomod-age/pkg/config"
	"github.com/fundament-oss/fundament/tools/gomod-age/pkg/proxy"
)

// Module represents a Go module dependency from `go list -m -json`.
type Module struct {
	Path     string  `json:"Path"`
	Version  string  `json:"Version"`
	Indirect bool    `json:"Indirect"`
	Main     bool    `json:"Main"`
	Replace  *Module `json:"Replace"`
}

// ResultKind categorizes how a module was handled.
type ResultKind string

const (
	KindPassed    ResultKind = "passed"
	KindViolation ResultKind = "violation"
	KindSkipped   ResultKind = "skipped"
	KindError     ResultKind = "error"
)

// ModuleResult holds the check result for a single module.
type ModuleResult struct {
	Module         string     `json:"module"`
	Version        string     `json:"version"`
	CheckedModule  string     `json:"checked_module,omitempty"`
	CheckedVersion string     `json:"checked_version,omitempty"`
	PublishTime    time.Time  `json:"publish_time,omitempty"`
	Age            string     `json:"age,omitempty"`
	Remaining      string     `json:"remaining,omitempty"`
	Reason         string     `json:"reason,omitempty"`
	Kind           ResultKind `json:"kind"`
}

// Results holds all check results grouped by kind.
type Results struct {
	Passed     []ModuleResult `json:"passed"`
	Violations []ModuleResult `json:"violations"`
	Skipped    []ModuleResult `json:"skipped"`
	Errors     []ModuleResult `json:"errors"`
}

// Check runs the age check on all dependencies.
func Check(ctx context.Context, cfg *config.Config, proxyClient *proxy.Client, now time.Time) (*Results, error) {
	modules, err := listModules(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing modules: %w", err)
	}

	proxyURL := os.Getenv("GOPROXY")

	results := &Results{}
	var mu sync.Mutex
	sem := make(chan struct{}, 10)
	var wg sync.WaitGroup

	for _, m := range modules {
		if m.Main {
			continue
		}
		if !cfg.Indirect && m.Indirect {
			continue
		}

		checkPath := m.Path
		checkVersion := m.Version
		var checkedModule, checkedVersion string

		if m.Replace != nil {
			if m.Replace.Version == "" {
				mu.Lock()
				results.Skipped = append(results.Skipped, ModuleResult{
					Module:  m.Path,
					Version: m.Version,
					Kind:    KindSkipped,
					Reason:  fmt.Sprintf("replaced with local path %s", m.Replace.Path),
				})
				mu.Unlock()
				continue
			}
			checkPath = m.Replace.Path
			checkVersion = m.Replace.Version
			checkedModule = checkPath
			checkedVersion = checkVersion
		}

		if cfg.IsIgnored(checkPath) {
			mu.Lock()
			results.Skipped = append(results.Skipped, ModuleResult{
				Module:  m.Path,
				Version: m.Version,
				Kind:    KindSkipped,
				Reason:  "ignored by pattern",
			})
			mu.Unlock()
			continue
		}

		if reason, ok := cfg.IsAllowed(checkPath, checkVersion); ok {
			r := "explicitly allowed"
			if reason != "" {
				r = fmt.Sprintf("explicitly allowed: %s", reason)
			}
			mu.Lock()
			results.Skipped = append(results.Skipped, ModuleResult{
				Module:  m.Path,
				Version: m.Version,
				Kind:    KindSkipped,
				Reason:  r,
			})
			mu.Unlock()
			continue
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(path, version, cMod, cVer string) {
			defer wg.Done()
			defer func() { <-sem }()

			publishTime, err := proxyClient.GetVersionTime(ctx, proxyURL, path, version)
			if err != nil {
				mu.Lock()
				results.Errors = append(results.Errors, ModuleResult{
					Module:  path,
					Version: version,
					Kind:    KindError,
					Reason:  err.Error(),
				})
				mu.Unlock()
				return
			}

			age := now.Sub(publishTime)
			mr := ModuleResult{
				Module:         path,
				Version:        version,
				CheckedModule:  cMod,
				CheckedVersion: cVer,
				PublishTime:    publishTime,
				Age:            FormatDuration(age),
			}

			if age < cfg.MinAge {
				mr.Kind = KindViolation
				mr.Remaining = FormatDuration(cfg.MinAge - age)
			} else {
				mr.Kind = KindPassed
			}

			mu.Lock()
			switch mr.Kind {
			case KindViolation:
				results.Violations = append(results.Violations, mr)
			default:
				results.Passed = append(results.Passed, mr)
			}
			mu.Unlock()
		}(checkPath, checkVersion, checkedModule, checkedVersion)
	}

	wg.Wait()
	return results, nil
}

// FormatDuration formats a duration in a human-readable way (e.g. "3d5h", "5h30m").
func FormatDuration(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd%dh", days, hours)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh%dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

func listModules(ctx context.Context) ([]Module, error) {
	cmd := exec.CommandContext(ctx, "go", "list", "-m", "-json", "all")
	cmd.Env = os.Environ()
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("running go list: %w", err)
	}

	var modules []Module
	dec := json.NewDecoder(bytes.NewReader(out))
	for dec.More() {
		var m Module
		if err := dec.Decode(&m); err != nil {
			return nil, fmt.Errorf("decoding module: %w", err)
		}
		modules = append(modules, m)
	}
	return modules, nil
}
