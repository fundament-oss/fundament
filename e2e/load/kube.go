package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os/exec"
	"strings"
)

// kube reaches the local deployment through kubectl and docker.
type kube struct {
	context   string
	namespace string
}

// newKube accepts only a k3d cluster on this machine: the tool seeds users,
// signs sessions and writes to the database.
func newKube(kubeContext, namespace string) (kube, error) {
	if !strings.HasPrefix(kubeContext, "k3d-") {
		return kube{}, fmt.Errorf("context %q is not a local k3d cluster", kubeContext)
	}
	return kube{context: kubeContext, namespace: namespace}, nil
}

// local checks that the context's API server listens on this machine and
// that its k3d node runs here in docker, so a context merely named k3d-*
// cannot point the tool elsewhere.
func (k kube) local(ctx context.Context) error {
	server, err := output(ctx, "kubectl", "config", "view", "--minify", "--context", k.context, "-o", "jsonpath={.clusters[0].cluster.server}")
	if err != nil {
		return err
	}
	u, err := url.Parse(server)
	if err != nil {
		return fmt.Errorf("context %s: server %q: %w", k.context, server, err)
	}
	ip := net.ParseIP(u.Hostname())
	onThisMachine := u.Hostname() == "localhost" || ip != nil && (ip.IsLoopback() || ip.IsUnspecified())
	if !onThisMachine {
		return fmt.Errorf("context %s talks to %s, not to this machine", k.context, server)
	}
	running, err := output(ctx, "docker", "inspect", "-f", "{{.State.Running}}", "k3d-"+k.cluster()+"-server-0")
	if err != nil || running != "true" {
		return fmt.Errorf("context %s: no running k3d node k3d-%s-server-0 on this machine", k.context, k.cluster())
	}
	return nil
}

// cluster is the k3d cluster's name, which also names its docker network and nodes.
func (k kube) cluster() string {
	return strings.TrimPrefix(k.context, "k3d-")
}

func (k kube) kubectlArgs(args ...string) []string {
	return append([]string{"--context", k.context, "-n", k.namespace}, args...)
}

func output(ctx context.Context, name string, args ...string) (string, error) {
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // kubectl, docker and mkcert, with arguments this tool builds
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}

func (k kube) kubectl(ctx context.Context, args ...string) (string, error) {
	return output(ctx, "kubectl", k.kubectlArgs(args...)...)
}

// primary is the pod of the database's current primary instance.
func (k kube) primary(ctx context.Context) (string, error) {
	pod, err := k.kubectl(ctx, "get", "pod", "-l", "cnpg.io/cluster=db,cnpg.io/instanceRole=primary", "-o", "name")
	if err != nil {
		return "", err
	}
	if pod == "" {
		return "", errors.New("no primary database pod")
	}
	return pod, nil
}

// psql is the command that runs psql as postgres on the primary, one row per
// line with comma-separated fields. app names the session in pg_stat_activity.
func (k kube) psql(pod, app string, args ...string) []string {
	return k.kubectlArgs(append([]string{"exec", "-i", pod, "-c", "postgres", "--", "psql", "-U", "postgres", "-d", "dbname=fundament application_name=" + app, "-At", "-F", ",", "-v", "ON_ERROR_STOP=1"}, args...)...)
}

func (k kube) sql(ctx context.Context, query string) (string, error) {
	pod, err := k.primary(ctx)
	if err != nil {
		return "", err
	}
	return output(ctx, "kubectl", k.psql(pod, "load", "-c", query)...)
}

// env is a variable set on a deployment's first container, empty when unset.
func (k kube) env(ctx context.Context, deployment, name string) (string, error) {
	return k.kubectl(ctx, "get", "deploy", deployment, "-o", fmt.Sprintf(`jsonpath={.spec.template.spec.containers[0].env[?(@.name==%q)].value}`, name))
}

func (k kube) secret(ctx context.Context, name, key string) (string, error) {
	encoded, err := k.kubectl(ctx, "get", "secret", name, "-o", fmt.Sprintf("jsonpath={.data.%s}", key))
	if err != nil {
		return "", err
	}
	value, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("secret %s/%s: %w", name, key, err)
	}
	return string(value), nil
}
