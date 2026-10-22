Feature: Project membership
  As an organization admin
  I want project membership limited to members of my organization
  So that users of other organizations are never provisioned on my clusters

  Background:
    Given I am authenticated as "alice@acme-corp.com"

  @api @project-member
  Scenario: A user of another organization cannot become a project member
    Given I have created a project named "e2e-outsider" on cluster "acme-cluster"
    When I add the user "david@globex.com" to the project with role "viewer"
    Then I should receive a failed precondition error
    And the error message should contain "must first be added to the organization"
    And the project should not have the member "david@globex.com"

  @api @project-member
  Scenario: A member of the organization can become a project member
    Given I have created a project named "e2e-insider" on cluster "acme-cluster"
    When I add the user "bart@acme-corp.com" to the project with role "viewer"
    Then I should not receive an error
    And the project should have the member "bart@acme-corp.com"
