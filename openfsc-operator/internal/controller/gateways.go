package controller

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openfscv1 "github.com/fundament-oss/fundament/openfsc-operator/api/v1"
	"github.com/fundament-oss/fundament/openfsc-operator/charts"
	"github.com/fundament-oss/fundament/openfsc-operator/internal/helm"
)

// registrations holds what the Controller Administration API reports, keyed by
// FSC gateway name. nil maps mean the API is not reachable yet (core still
// coming up); gateways are then provisioned but reported Pending.
type registrations struct {
	inways  map[string]string // name -> registered address
	outways map[string]bool
}

func (r *FSCInstallationReconciler) observeRegistrations(ctx context.Context, inst *openfscv1.FSCInstallation) (registrations, error) {
	api, err := r.Admin.forNamespace(ctx, inst.Namespace)
	if err != nil {
		if errors.Is(err, errAdminNotConfigured) {
			return registrations{}, nil
		}
		return registrations{}, err
	}
	inways, err := api.ListInways(ctx)
	if err != nil {
		// The cached client may hold a renewed-away certificate; rebuild it
		// from the current Secret on the next attempt.
		r.Admin.forget(inst.Namespace)
		return registrations{}, fmt.Errorf("list registered inways: %w", err)
	}
	outways, err := api.ListOutways(ctx)
	if err != nil {
		r.Admin.forget(inst.Namespace)
		return registrations{}, fmt.Errorf("list registered outways: %w", err)
	}

	reg := registrations{inways: map[string]string{}, outways: map[string]bool{}}
	for _, iw := range inways {
		reg.inways[iw.Name] = iw.Address
	}
	for _, ow := range outways {
		reg.outways[ow.Name] = true
	}
	return reg, nil
}

func (r *FSCInstallationReconciler) ensureGateways(ctx context.Context, inst *openfscv1.FSCInstallation, helmClient *helm.Client, reg registrations) ([]openfscv1.GatewayStatus, []openfscv1.GatewayStatus, bool, error) {
	allActive := true

	inways := make([]openfscv1.GatewayStatus, 0, len(inst.Spec.Inways))
	for _, gw := range inst.Spec.Inways {
		entry, err := r.ensureInway(ctx, inst, helmClient, gw, reg)
		if err != nil {
			return nil, nil, false, fmt.Errorf("inway %s: %w", gw.Name, err)
		}
		preserveLastSynced(&entry, inst.Status.Inways)
		allActive = allActive && entry.Phase == openfscv1.PhaseActive
		inways = append(inways, entry)
	}

	outways := make([]openfscv1.GatewayStatus, 0, len(inst.Spec.Outways))
	for _, gw := range inst.Spec.Outways {
		entry, err := r.ensureOutway(ctx, inst, helmClient, gw, reg)
		if err != nil {
			return nil, nil, false, fmt.Errorf("outway %s: %w", gw.Name, err)
		}
		preserveLastSynced(&entry, inst.Status.Outways)
		allActive = allActive && entry.Phase == openfscv1.PhaseActive
		outways = append(outways, entry)
	}

	if err := r.sweepOrphanGateways(ctx, inst, helmClient); err != nil {
		return nil, nil, false, err
	}
	return inways, outways, allActive, nil
}

func (r *FSCInstallationReconciler) ensureInway(ctx context.Context, inst *openfscv1.FSCInstallation, helmClient *helm.Client, gw openfscv1.InwayConfig, reg registrations) (openfscv1.GatewayStatus, error) {
	release := inwayRelease(gw.Name)

	var extraHosts []string
	if gw.SelfAddress != "" {
		if u, err := url.Parse(gw.SelfAddress); err == nil && u.Hostname() != "" {
			extraHosts = append(extraHosts, u.Hostname())
		}
	}
	ready, err := r.provisionGateway(ctx, inst, helmClient, release, extraHosts, inwayValues(inst, gw))
	if err != nil {
		return openfscv1.GatewayStatus{}, err
	}
	entry := openfscv1.GatewayStatus{Name: gw.Name}
	switch {
	case !ready:
		setGatewayPending(&entry, "waiting for cert-manager to issue the gateway certificates")
	case reg.inways == nil:
		setGatewayPending(&entry, "inway deployed; the Controller Administration API is not reachable yet")
	default:
		addr, registered := reg.inways[gw.Name]
		if registered {
			setGatewayActive(&entry, fmt.Sprintf("registered at %s", addr))
			entry.URL = addr
		} else {
			setGatewayPending(&entry, "inway deployed; waiting for it to register with the Controller")
		}
	}
	return entry, nil
}

func (r *FSCInstallationReconciler) ensureOutway(ctx context.Context, inst *openfscv1.FSCInstallation, helmClient *helm.Client, gw openfscv1.OutwayConfig, reg registrations) (openfscv1.GatewayStatus, error) {
	release := outwayRelease(gw.Name)

	ready, err := r.provisionGateway(ctx, inst, helmClient, release, nil, outwayValues(inst, gw))
	if err != nil {
		return openfscv1.GatewayStatus{}, err
	}
	entry := openfscv1.GatewayStatus{Name: gw.Name, URL: outwayURL(inst.Namespace, gw)}
	switch {
	case !ready:
		setGatewayPending(&entry, "waiting for cert-manager to issue the gateway certificates")
	case reg.outways == nil:
		setGatewayPending(&entry, "outway deployed; the Controller Administration API is not reachable yet")
	case reg.outways[gw.Name]:
		setGatewayActive(&entry, "registered with the Controller")
	default:
		setGatewayPending(&entry, "outway deployed; waiting for it to register with the Controller")
	}
	return entry, nil
}

// provisionGateway applies the gateway's certificates and installs its Helm
// release. The release is (re)installed when missing, when the spec changed,
// or when the vendored chart version moved (an operator upgrade). It reports
// false while the certificates are still being issued; the release is
// installed regardless — its pod waits on the secret mounts.
func (r *FSCInstallationReconciler) provisionGateway(ctx context.Context, inst *openfscv1.FSCInstallation, helmClient *helm.Client, release string, extraHosts []string, values map[string]string) (bool, error) {
	deployedVersion, err := helmClient.DeployedChartVersion(release)
	if err != nil {
		return false, fmt.Errorf("check release %s: %w", release, err)
	}
	outdated := deployedVersion != charts.Version || inst.Generation != inst.Status.ObservedGeneration

	certsReady, err := ensureGatewayCerts(ctx, r.Direct, inst, release, extraHosts, outdated)
	if err != nil {
		return false, err
	}
	if outdated {
		chrt, err := loadGatewayChart(release)
		if err != nil {
			return false, fmt.Errorf("load chart for %s: %w", release, err)
		}
		vals, err := helm.SetValues(values)
		if err != nil {
			return false, fmt.Errorf("render values for %s: %w", release, err)
		}
		if err := helmClient.UpgradeInstall(ctx, release, chrt, vals); err != nil {
			return false, fmt.Errorf("install release %s: %w", release, err)
		}
	}
	return certsReady, nil
}

// sweepOrphanGateways uninstalls gateway releases whose spec entry is gone
// (renamed or removed). Only releases installed from the vendored gateway
// charts are touched: release names are user-reachable, so a coincidentally
// named user release must survive.
func (r *FSCInstallationReconciler) sweepOrphanGateways(ctx context.Context, inst *openfscv1.FSCInstallation, helmClient *helm.Client) error {
	declared := map[string]bool{}
	for _, gw := range inst.Spec.Inways {
		declared[inwayRelease(gw.Name)] = true
	}
	for _, gw := range inst.Spec.Outways {
		declared[outwayRelease(gw.Name)] = true
	}

	for _, prefix := range []string{inwayRelease(""), outwayRelease("")} {
		releases, err := helmClient.List(prefix)
		if err != nil {
			return fmt.Errorf("list gateway releases: %w", err)
		}
		for _, release := range releases {
			if declared[release.Name] || release.ChartName != gatewayChartName(release.Name) {
				continue
			}
			if err := helmClient.Uninstall(release.Name); err != nil {
				return fmt.Errorf("uninstall orphaned release %s: %w", release.Name, err)
			}
			if err := deleteGatewayCerts(ctx, r.Direct, inst.Namespace, release.Name); err != nil {
				return err
			}
		}
	}
	return nil
}

func setGatewayActive(entry *openfscv1.GatewayStatus, msg string) {
	entry.Phase = openfscv1.PhaseActive
	entry.Message = msg
	now := metav1.NewTime(time.Now())
	entry.LastSyncedTime = &now
}

func setGatewayPending(entry *openfscv1.GatewayStatus, msg string) {
	entry.Phase = openfscv1.PhasePending
	entry.Message = msg
}

// preserveLastSynced keeps the previous LastSyncedTime when the gateway was
// already Active, so steady-state reconciles do not produce status churn.
func preserveLastSynced(entry *openfscv1.GatewayStatus, prev []openfscv1.GatewayStatus) {
	if entry.Phase != openfscv1.PhaseActive {
		return
	}
	for _, p := range prev {
		if p.Name == entry.Name && p.Phase == openfscv1.PhaseActive && p.LastSyncedTime != nil {
			entry.LastSyncedTime = p.LastSyncedTime
			return
		}
	}
}
