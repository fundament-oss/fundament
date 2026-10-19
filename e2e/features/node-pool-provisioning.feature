# Runs only against a real Gardener (just cluster-worker gardener-up + just dev -p local-gardener).
# Requires: a running cluster (E2E_CLUSTER_NAME, default "tf-a") owned by the
# API key's organization, kubectl access to the seed kind cluster
# (E2E_SEED_CONTEXT, default "kind-gardener-operator-local"), and patience:
# Gardener timing makes this a 10-20 minute test.
# Run: just e2e test-real-gardener
#
# (Not `just e2e test-tags @real-gardener`: the default cucumber profile
# excludes @real-gardener, and cucumber-js ANDs that exclusion with any
# --tags given on the CLI, so that invocation always matches zero scenarios.
# test-real-gardener uses a dedicated cucumber.json profile instead.)
@real-gardener
Feature: Node pool provisioning failures are reported

  Scenario: A node pool with no available machines reports waiting, and recovers
    Given a running cluster is available
    And the seed has no free machine capacity
    When I add a node pool named "nomach" with autoscale 1 to 1
    Then within 10 minutes the node pool "nomach" reports waiting for machines with a reason
    And the cluster status is "UNHEALTHY" or "UPGRADING"
    And the activity log contains a "nodepool_waiting" event
    When the seed capacity is restored
    Then within 15 minutes the node pool "nomach" reports a healthy or provisioning state
    And the activity log contains a "nodepool_ready" event
    And the cluster status becomes "RUNNING"
