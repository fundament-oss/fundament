Feature: API Key Management
  As a Fundament user
  I want to manage API keys programmatically
  So that I can enable automated access to the platform

  Background:
    Given I am authenticated as "alice@acme-corp.com"

  @api @apikey @smoke
  Scenario: Create a new API key
    When I create an API key with name "test-key"
    Then the response should contain a token starting with "fun_"
    And the token should be 40 characters long
    And the response should contain a token prefix of 8 characters
    And the API key should appear in the list of keys

  @api @apikey
  Scenario: Create API key with expiration
    When I create an API key with name "expiring-key" expiring in "720h"
    Then the API key should have an expiration date
    And the API key should be active

  @api @apikey
  Scenario: List API keys returns masked tokens
    Given I have created an API key named "list-test-key"
    When I list all API keys
    Then I should see the API key "list-test-key" in the list
    And the API key should have a token prefix but not the full token

  @api @apikey
  Scenario: Get API key details
    Given I have created an API key named "detail-key"
    When I get the API key by ID
    Then I should see the key name "detail-key"
    And I should see a created timestamp
    And I should NOT see the full token

  @api @apikey
  Scenario: Revoke an API key
    Given I have created an API key named "revoke-test"
    When I revoke the API key
    Then the API key should have a revoked timestamp
    And the API key should still appear in the list

  @api @apikey
  Scenario: Delete an API key
    Given I have created an API key named "delete-test"
    When I delete the API key
    Then the API key should not appear in the list
    And getting the API key by ID should return not found

  @api @apikey @negative
  Scenario: Create API key with duplicate name fails
    Given I have created an API key named "duplicate-test"
    When I try to create another API key with name "duplicate-test"
    Then I should receive an error

  @api @apikey @negative
  Scenario: Create API key without authentication fails
    Given I have no authentication
    When I try to create an API key with name "unauth-test"
    Then I should receive an unauthenticated error

  @api @apikey @negative
  Scenario: Revoke already revoked API key fails
    Given I have created an API key named "double-revoke-test"
    And I have revoked the API key
    When I try to revoke the API key again
    Then I should receive a not found error

  @api @apikey @negative
  Scenario: Delete non-existent API key fails
    When I try to delete an API key with ID "00000000-0000-0000-0000-000000000000"
    Then I should receive a not found error
