package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/fundament-oss/fundament/common/kubename"
	"github.com/fundament-oss/fundament/functl/pkg/client"
	"github.com/fundament-oss/fundament/functl/pkg/plugindev"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

// PluginDevCmd runs a plugin with hot reload in the local sandbox cluster.
type PluginDevCmd struct {
	Dir     string            `arg:"" optional:"" default:"." help:"Plugin directory (holds definition.yaml)."`
	Cleanup bool              `default:"true" negatable:"" help:"Delete the dev installation on exit."`
	Set     map[string]string `mapsep:"none" placeholder:"KEY=VALUE" help:"Config value for the installation's spec.config; repeatable, overrides --config."`
	Config  string            `type:"existingfile" help:"YAML file of config values (KEY: value) for the installation's spec.config."`
}

// PluginDevDownCmd removes a plugin's dev installation from the sandbox.
type PluginDevDownCmd struct {
	Dir string `arg:"" optional:"" default:"." help:"Plugin directory (holds definition.yaml)."`
}

// detectNamed runs Detect and rejects a definition without name or version.
func detectNamed(dir string) (plugindev.Layout, error) {
	l, err := plugindev.Detect(dir)
	if err != nil {
		return l, err
	}
	if l.Name == "" {
		return l, errors.New("definition.yaml has no metadata.name")
	}
	if errs := validation.IsDNS1123Label(l.Name); len(errs) > 0 {
		return l, fmt.Errorf("definition.yaml metadata.name %q is not a valid DNS label: %s", l.Name, strings.Join(errs, "; "))
	}
	if strings.Contains(l.Name, "--") {
		return l, fmt.Errorf("definition.yaml metadata.name %q must not contain \"--\": it separates organization and plugin in installation names", l.Name)
	}
	if l.Version == "" {
		return l, errors.New("definition.yaml has no metadata.version")
	}
	return l, nil
}

// orgClient returns an API client for the active organization, with its id and machine name.
func orgClient(ctx context.Context, cliCtx *Context) (apiClient *client.Client, id, name string, err error) {
	id, err = activeOrg(cliCtx)
	if err != nil {
		return nil, "", "", err
	}
	apiClient, err = NewClientFromConfig(WithOrg(id))
	if err != nil {
		return nil, "", "", err
	}
	resp, err := apiClient.Organizations().GetOrganization(ctx, organizationv1.GetOrganizationRequest_builder{Id: id}.Build())
	if err != nil {
		return nil, "", "", fmt.Errorf("get organization: %w", err)
	}
	return apiClient, id, resp.GetOrganization().GetName(), nil
}

// devSession is what the dev loop needs once startup has built and published the plugin.
type devSession struct {
	layout       plugindev.Layout
	target       plugindev.Target
	cluster      plugindev.Cluster
	installation string
	renderDir    string
}

// Run executes the plugin dev command.
func (c *PluginDevCmd) Run(ctx *Context) error {
	l, err := detectNamed(c.Dir)
	if err != nil {
		return err
	}
	src, err := os.ReadFile(filepath.Join(l.PluginDir, "definition.yaml"))
	if err != nil {
		return fmt.Errorf("read definition: %w", err)
	}
	config, err := plugindev.ResolveConfig(src, c.Config, c.Set)
	if err != nil {
		return err
	}

	runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()

	s, err := c.start(runCtx, ctx, l, src, config)
	if runCtx.Err() != nil {
		// Interrupted before anything was installed.
		return nil
	}
	if err != nil {
		return err
	}

	fmt.Println("Note: changes to definition.yaml or the config values are NOT hot-reloaded — restart functl plugin dev.")
	if l.SDKReplace != "" {
		fmt.Printf("Note: edits to %s are NOT hot-reloaded — restart functl plugin dev.\n", l.SDKReplace)
	}

	namespace := kubename.PluginNamespace(s.installation)
	devErr := plugindev.Dev(runCtx, s.renderDir, s.layout, s.target, namespace, c.Cleanup)
	if !c.Cleanup {
		return devErr
	}
	checkCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return errors.Join(devErr, plugindev.CheckCleanedUp(checkCtx, s.cluster, s.installation))
}

// start runs the startup checks, builds the image, publishes the dev version
// and renders the files skaffold dev needs.
func (c *PluginDevCmd) start(runCtx context.Context, ctx *Context, l plugindev.Layout, src []byte, config map[string]string) (devSession, error) {
	apiClient, orgID, org, err := orgClient(runCtx, ctx)
	if err != nil {
		return devSession{}, err
	}
	pubClient, err := publicationClient(ctx, apiClient)
	if err != nil {
		return devSession{}, err
	}
	target := plugindev.Sandbox

	if err := plugindev.CheckTools(l, exec.LookPath, plugindev.DockerInfo); err != nil {
		return devSession{}, err
	}

	cluster, err := plugindev.NewCluster(target)
	if err != nil {
		return devSession{}, err
	}
	if err := plugindev.CheckCluster(runCtx, cluster, target, orgID); err != nil {
		return devSession{}, err
	}

	installation := plugindev.InstallationName(org, l.Name)
	if err := plugindev.WaitIfTerminating(runCtx, cluster, installation, os.Stdout, plugindev.TeardownPollInterval, plugindev.TeardownTimeout); err != nil {
		return devSession{}, err
	}
	if err := plugindev.CheckVersionBump(runCtx, cluster, installation, devVersion(l.Version)); err != nil {
		return devSession{}, err
	}
	if err := l.ResolveSourceDirs(); err != nil {
		return devSession{}, err
	}
	if err := plugindev.BuildConsole(runCtx, l); err != nil {
		return devSession{}, err
	}

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return devSession{}, fmt.Errorf("resolve cache dir: %w", err)
	}
	renderDir := filepath.Join(cacheDir, "functl", "plugin-dev", installation)
	if err := os.MkdirAll(renderDir, 0o755); err != nil { //nolint:gosec // cache dir, not secret
		return devSession{}, fmt.Errorf("create %s: %w", renderDir, err)
	}
	params := plugindev.RenderParams{Layout: l, Target: target, OrgName: org, Config: config}
	if err := plugindev.Render(renderDir, params); err != nil {
		return devSession{}, err
	}

	image, err := plugindev.BuildImage(runCtx, renderDir, target)
	if err != nil {
		return devSession{}, err
	}

	published, err := publishVersion(runCtx, pubClient, src, image, publishOptions{Create: true, Dev: true})
	if err != nil {
		return devSession{}, err
	}
	if err := plugindev.RenderInstallation(renderDir, params, published.GetVersion()); err != nil {
		return devSession{}, err
	}
	return devSession{layout: l, target: target, cluster: cluster, installation: installation, renderDir: renderDir}, nil
}

// Run executes the plugin dev-down command.
func (c *PluginDevDownCmd) Run(ctx *Context) error {
	l, err := detectNamed(c.Dir)
	if err != nil {
		return err
	}
	_, _, org, err := orgClient(context.Background(), ctx)
	if err != nil {
		return err
	}
	cluster, err := plugindev.NewCluster(plugindev.Sandbox)
	if err != nil {
		return err
	}
	installation := plugindev.InstallationName(org, l.Name)
	return plugindev.DevDown(context.Background(), cluster, installation, os.Stdout, plugindev.TeardownPollInterval, plugindev.TeardownTimeout)
}
