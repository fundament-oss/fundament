# Runs only against a real Gardener, in the test-env VM: just test-env test @project-rbac
# Uses the baseline cluster (E2E_CLUSTER_NAME, default "e2e-base"; test-env sets it).
@real-gardener @project-rbac @api
Feature: Project roles on a real cluster
  Project members get the built-in admin or view role in each namespace of
  their project on the cluster (FUN-7), reached through the kube-api-proxy.
  Bart and Cindy are viewers of the organization, which gives them no access
  to the cluster of its own: what they can do comes from the project role.

  Background:
    Given I am authenticated as "alice@acme-corp.com"
    And a new project on the baseline cluster with a namespace "apps"

  Scenario: A viewer reads the project namespace but cannot write to it
    When I add "bart@acme-corp.com" to the project as a viewer
    Then within 5 minutes "bart@acme-corp.com" can list config maps in "apps"
    And "bart@acme-corp.com" cannot create a config map in "apps"

  # docs/user/clusters.md: a user who is neither an organization admin nor a
  # project member gets no ServiceAccount, and their requests are refused.
  Scenario: A user outside the project is refused
    Then "cindy@acme-corp.com" cannot list config maps in "apps"

  Scenario: A project admin writes to the project namespace
    When I add "cindy@acme-corp.com" to the project as an admin
    Then within 5 minutes "cindy@acme-corp.com" can create a config map in "apps"

  Scenario: A member removed from the project loses access
    When I add "bart@acme-corp.com" to the project as a viewer
    Then within 5 minutes "bart@acme-corp.com" can list config maps in "apps"
    When I remove "bart@acme-corp.com" from the project
    Then within 5 minutes "bart@acme-corp.com" can no longer list config maps in "apps"
