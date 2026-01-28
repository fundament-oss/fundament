Feature: API Key User Isolation
  As a Fundament platform
  I want to ensure API keys are isolated per user
  So that users cannot access other users' API keys

  Background:
    Given I am authenticated as "alice@acme-corp.com"

  @api @apikey @isolation
  Scenario: User cannot see other user's API keys in list
    Given I have created an API key named "user-a-key"
    When I switch to user "bob@acme-corp.com"
    And I list all API keys
    Then I should NOT see the API key "user-a-key" in the list

  @api @apikey @isolation
  Scenario: User cannot get other user's API key by ID
    Given I have created an API key named "private-key"
    And I save the API key ID
    When I switch to user "bob@acme-corp.com"
    And I try to get the saved API key by ID
    Then I should receive a not found error

  @api @apikey @isolation
  Scenario: User cannot revoke other user's API key
    Given I have created an API key named "no-revoke-key"
    And I save the API key ID
    When I switch to user "bob@acme-corp.com"
    And I try to revoke the saved API key
    Then I should receive a not found error

  @api @apikey @isolation
  Scenario: User cannot delete other user's API key
    Given I have created an API key named "no-delete-key"
    And I save the API key ID
    When I switch to user "bob@acme-corp.com"
    And I try to delete the saved API key
    Then I should receive a not found error

  @api @apikey @isolation
  Scenario: Each user sees only their own API keys
    Given I have created an API key named "admin-key-1"
    And I have created an API key named "admin-key-2"
    When I switch to user "bob@acme-corp.com"
    And I create an API key with name "member-key-1"
    And I list all API keys
    Then I should see the API key "member-key-1" in the list
    And I should NOT see the API key "admin-key-1" in the list
    And I should NOT see the API key "admin-key-2" in the list
