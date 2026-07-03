package controller

import (
	"fmt"
	"strings"

	chart "helm.sh/helm/v4/pkg/chart/v2"

	"github.com/fundament-oss/fundament/openfsc-operator/charts"
	"github.com/fundament-oss/fundament/openfsc-operator/internal/helm"
)

const umbrellaChartName = "open-fsc"

// The charts are loaded fresh per install: Helm mutates chart structs while
// rendering (dependency pruning, values merging), so they must not be shared
// across releases.

func loadUmbrellaChart() (*chart.Chart, error) {
	data, err := charts.FS.ReadFile(charts.UmbrellaArchive)
	if err != nil {
		return nil, fmt.Errorf("read umbrella chart: %w", err)
	}
	chrt, err := helm.LoadArchive(data)
	if err != nil {
		return nil, fmt.Errorf("load umbrella chart: %w", err)
	}
	return chrt, nil
}

func loadGatewayChart(release string) (*chart.Chart, error) {
	chrt, err := helm.LoadDir(charts.FS, gatewayChartName(release))
	if err != nil {
		return nil, fmt.Errorf("load gateway chart: %w", err)
	}
	return chrt, nil
}

// gatewayChartName is the chart a gateway release must come from. The chart
// directory names equal the chart names, and the orphan sweep and teardown
// only uninstall releases installed from these charts.
func gatewayChartName(release string) string {
	if strings.HasPrefix(release, outwayRelease("")) {
		return charts.OutwayDir
	}
	return charts.InwayDir
}
