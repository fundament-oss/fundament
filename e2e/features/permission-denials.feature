Feature: Permission denials
  As a user without the role an action needs
  I want a denial at once, naming the role, and no button for it
  So that I know why I cannot do something and who can

  # Uses the acme-corp seed (db/testdata): alice is an organization admin,
  # bart is an organization viewer and admin of acme-project, cindy is an
  # organization viewer and viewer of acme-project.

  @api @permission-denials
  Scenario Outline: A viewer is denied at once, with the role the action needs
    Given I am authenticated as "bart@acme-corp.com"
    And my access to the seeded cluster and project has synced
    When I call "<rpc>" on the seeded cluster
    Then the call should be denied in under 2 seconds
    And the denial should name the action "<action>" and the role "<role>"

    Examples:
      | rpc           | action         | role               |
      | DeleteCluster | DELETE_CLUSTER | ORGANIZATION_ADMIN |
      | CreateProject | CREATE_PROJECT | ORGANIZATION_ADMIN |
      | QueryLogs     | VIEW_LOGS      | ORGANIZATION_ADMIN |

  @api @permission-denials
  Scenario: A project viewer is denied creating a namespace, with the role it needs
    Given I am authenticated as "cindy@acme-corp.com"
    And my access to the seeded cluster and project has synced
    When I call "CreateNamespace" on the seeded project
    Then the call should be denied in under 2 seconds
    And the denial should name the action "CREATE_NAMESPACE" and the role "PROJECT_ADMIN"

  @api @permission-denials
  Scenario: CheckPermissions tells a viewer what the API enforces
    Given I am authenticated as "bart@acme-corp.com"
    Then asking whether I may "CREATE_NAMESPACE" on the seeded project should answer true
    And asking whether I may "DELETE_CLUSTER" on the seeded cluster should answer false

  @api @permission-denials
  Scenario: CheckPermissions tells an organization admin they may delete a cluster
    Given I am authenticated as "alice@acme-corp.com"
    Then asking whether I may "DELETE_CLUSTER" on the seeded cluster should answer true

  # The UI scenarios carry @api for the API token: it waits for the user's
  # access to reach OpenFGA before the console is asked anything.
  @api @ui @permission-denials
  Scenario: A viewer sees no actions that need an admin
    Given I am authenticated as "bart@acme-corp.com"
    And my access to the seeded cluster and project has synced
    And I am signed in to the console as "bart@acme-corp.com"
    When I open the seeded cluster in the console
    And the console has had its permission answers and its re-asks
    Then I should not see the action "Delete cluster"
    And I should not see the action "New project…"
    And I should not see the action "Invite member…"

  @api @ui @permission-denials
  Scenario: A viewer sees why the logs are not available
    Given I am authenticated as "bart@acme-corp.com"
    And my access to the seeded cluster and project has synced
    And I am signed in to the console as "bart@acme-corp.com"
    When I open the logs in the console
    Then I should see the message "Logs are available to organization admins"

  @api @ui @permission-denials
  Scenario: A project viewer sees no way to create a namespace
    Given I am authenticated as "cindy@acme-corp.com"
    And my access to the seeded cluster and project has synced
    And I am signed in to the console as "cindy@acme-corp.com"
    When I open the namespaces of the seeded project in the console
    And the console has had its permission answers and its re-asks
    Then I should not see the action "New namespace…"

  @api @ui @permission-denials
  Scenario: An organization admin sees every action
    Given I am authenticated as "alice@acme-corp.com"
    And my access to the seeded cluster and project has synced
    And I am signed in to the console as "alice@acme-corp.com"
    When I open the seeded cluster in the console
    Then I should see the action "Delete cluster"
    And I should see the action "New project…"
    And I should see the action "Invite member…"
