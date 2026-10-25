# Runs only against a real Gardener, in the test-env VM: just test-env test @real-gardener
# Requires: a running cluster (E2E_CLUSTER_NAME, default "e2e-base"; test-env sets it) owned by the
# API key's organization, kubectl access to the seed kind cluster
# (E2E_SEED_CONTEXT, default "kind-gardener-operator-local"), and patience: Gardener
# reports a pool without machines only when its wait for them times out, about
# 10 minutes in, so this is a 15-35 minute test.
@real-gardener
Feature: Node pool provisioning on a real cluster

  Scenario: A node pool with no available machines reports waiting, recovers, and leaves with its node
    Given a running cluster is available
    And the seed has no free machine capacity
    When I add a node pool named "nomach" with autoscale 1 to 1
    Then within 20 minutes the node pool "nomach" reports waiting for machines with a reason
    And the cluster status is "UNHEALTHY" or "UPGRADING"
    And the activity log contains a "nodepool_waiting" event
    When the seed capacity is restored
    Then within 15 minutes the node pool "nomach" reports a healthy state
    And the activity log contains a "nodepool_ready" event
    And the cluster status becomes "RUNNING"
    And the cluster has 1 node in the node pool "nomach"
    When I delete the node pool "nomach"
    Then within 15 minutes the node pool "nomach" is no longer listed
    And within 5 minutes the cluster has no node in the node pool "nomach"
