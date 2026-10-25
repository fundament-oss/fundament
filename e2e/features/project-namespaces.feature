# Runs only against a real Gardener, in the test-env VM: just test-env test @project-namespaces
# Uses the baseline cluster (E2E_CLUSTER_NAME, default "e2e-base"; test-env sets it).
@real-gardener @project-namespaces @api
Feature: Project namespaces on a real cluster
  Each namespace of a project exists on the project's cluster as
  tnt-<project>--<namespace>, created and removed by cluster-worker.

  Background:
    Given I am authenticated as "alice@acme-corp.com"
    And a new project on the baseline cluster with a namespace "apps"

  Scenario: A namespace added to a project appears on the cluster, and leaves with its deletion
    Then within 5 minutes the cluster has the namespace "apps"
    When I add the namespace "jobs" to the project
    Then within 5 minutes the cluster has the namespace "jobs"
    When I delete the namespace "jobs" from the project
    Then within 5 minutes the cluster no longer has the namespace "jobs"
    And within 1 minutes the cluster has the namespace "apps"
