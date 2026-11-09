# Runs only against a real Gardener, in the test-env VM: just test-env test @cluster-observability
# Uses the baseline cluster (E2E_CLUSTER_NAME, default "e2e-base"; test-env sets it).
@real-gardener @cluster-observability @api
Feature: Metrics and logs of a real cluster
  organization-api answers metrics and log queries for a cluster from the
  Prometheus and Vali Gardener runs for its shoot.

  Background:
    Given I am authenticated as "alice@acme-corp.com"

  Scenario: The cluster's workload metrics list its nodes and namespaces
    Then within 5 minutes the metrics of the baseline cluster list its nodes and the namespace "kube-system"

  Scenario: The cluster's logs hold lines of its system pods
    Then within 5 minutes the logs of the baseline cluster hold lines from the namespace "kube-system"
