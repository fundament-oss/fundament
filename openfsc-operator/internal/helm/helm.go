// Package helm wraps the Helm SDK (helm.sh/helm/v3 as a library) for the
// operator's chart installs. Releases use the default Secret storage driver,
// so they are fully interoperable with the helm CLI.
package helm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/storage/driver"
	"helm.sh/helm/v3/pkg/strvals"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/yaml"
)

// Client runs Helm actions in a single namespace (release state and
// resources). Not safe for concurrent use; create one per reconcile.
type Client struct {
	namespace string
	cfg       *action.Configuration
}

func NewClient(namespace string) *Client {
	return &Client{namespace: namespace}
}

// config builds the action.Configuration once per Client. The in-cluster (or
// kubeconfig) REST config is resolved by cli-runtime's default loading rules.
func (c *Client) config() (*action.Configuration, error) {
	if c.cfg != nil {
		return c.cfg, nil
	}
	flags := genericclioptions.NewConfigFlags(false)
	flags.Namespace = &c.namespace
	cfg := new(action.Configuration)
	debug := func(format string, v ...any) { slog.Debug(fmt.Sprintf(format, v...), "helm", true) }
	if err := cfg.Init(flags, c.namespace, "", debug); err != nil {
		return nil, fmt.Errorf("init helm configuration: %w", err)
	}
	c.cfg = cfg
	return cfg, nil
}

// DeployedChartVersion returns the chart version of the release's latest
// revision, or "" when the release does not exist.
func (c *Client) DeployedChartVersion(release string) (string, error) {
	cfg, err := c.config()
	if err != nil {
		return "", err
	}
	rel, err := action.NewGet(cfg).Run(release)
	if errors.Is(err, driver.ErrReleaseNotFound) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("helm get %s: %w", release, err)
	}
	if rel.Chart == nil || rel.Chart.Metadata == nil {
		return "", nil
	}
	return rel.Chart.Metadata.Version, nil
}

// ReleaseInfo identifies a release and the chart it was installed from.
type ReleaseInfo struct {
	Name      string
	ChartName string
}

// List returns the releases in the client's namespace with the given name
// prefix, in any state (so half-uninstalled releases still show up).
func (c *Client) List(prefix string) ([]ReleaseInfo, error) {
	cfg, err := c.config()
	if err != nil {
		return nil, err
	}
	list := action.NewList(cfg)
	list.StateMask = action.ListAll
	releases, err := list.Run()
	if err != nil {
		return nil, fmt.Errorf("helm list: %w", err)
	}
	var infos []ReleaseInfo
	for _, rel := range releases {
		if !strings.HasPrefix(rel.Name, prefix) {
			continue
		}
		info := ReleaseInfo{Name: rel.Name}
		if rel.Chart != nil && rel.Chart.Metadata != nil {
			info.ChartName = rel.Chart.Metadata.Name
		}
		infos = append(infos, info)
	}
	return infos, nil
}

// UpgradeInstall installs the chart as the named release, or upgrades it when
// it already exists (the SDK equivalent of `helm upgrade --install`, without
// --wait: the reconciler tracks readiness instead of blocking a worker).
func (c *Client) UpgradeInstall(ctx context.Context, release string, chrt *chart.Chart, values map[string]any) error {
	cfg, err := c.config()
	if err != nil {
		return err
	}
	hist := action.NewHistory(cfg)
	hist.Max = 1
	_, err = hist.Run(release)
	if errors.Is(err, driver.ErrReleaseNotFound) {
		install := action.NewInstall(cfg)
		install.ReleaseName = release
		install.Namespace = c.namespace
		// Never create the namespace: releases land in the installation's own
		// namespace, which exists by definition and belongs to the team.
		install.CreateNamespace = false
		if _, err := install.RunWithContext(ctx, chrt, values); err != nil {
			return fmt.Errorf("helm install %s: %w", release, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("helm history %s: %w", release, err)
	}
	upgrade := action.NewUpgrade(cfg)
	upgrade.Namespace = c.namespace
	if _, err := upgrade.RunWithContext(ctx, release, chrt, values); err != nil {
		return fmt.Errorf("helm upgrade %s: %w", release, err)
	}
	return nil
}

// Uninstall removes the release; a missing release is not an error.
func (c *Client) Uninstall(release string) error {
	cfg, err := c.config()
	if err != nil {
		return err
	}
	uninstall := action.NewUninstall(cfg)
	if _, err := uninstall.Run(release); err != nil && !errors.Is(err, driver.ErrReleaseNotFound) {
		return fmt.Errorf("helm uninstall %s: %w", release, err)
	}
	return nil
}

func LoadArchive(data []byte) (*chart.Chart, error) {
	chrt, err := loader.LoadArchive(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("load chart archive: %w", err)
	}
	return chrt, nil
}

func LoadDir(fsys fs.FS, dir string) (*chart.Chart, error) {
	var files []*loader.BufferedFile
	err := fs.WalkDir(fsys, dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		files = append(files, &loader.BufferedFile{
			Name: strings.TrimPrefix(path, dir+"/"),
			Data: data,
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read chart dir %s: %w", dir, err)
	}
	chrt, err := loader.LoadFiles(files)
	if err != nil {
		return nil, fmt.Errorf("load chart %s: %w", dir, err)
	}
	return chrt, nil
}

func ParseValues(data []byte) (map[string]any, error) {
	vals := map[string]any{}
	if err := yaml.Unmarshal(data, &vals); err != nil {
		return nil, fmt.Errorf("parse values: %w", err)
	}
	return vals, nil
}

// ApplySet overlays --set-style key=value pairs onto vals, in sorted key
// order for determinism.
func ApplySet(vals map[string]any, set map[string]string) error {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if err := strvals.ParseInto(k+"="+set[k], vals); err != nil {
			return fmt.Errorf("parse set %q: %w", k, err)
		}
	}
	return nil
}

func SetValues(set map[string]string) (map[string]any, error) {
	vals := map[string]any{}
	if err := ApplySet(vals, set); err != nil {
		return nil, err
	}
	return vals, nil
}
