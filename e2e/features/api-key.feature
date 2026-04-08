Feature: API Key Management
  As a user of Fundament
  I want to manage API key lifecycle
  So that I can delete keys in any state

  Background:
    Given I am authenticated as "alice@acme-corp.com"

  @api @apikey
  Scenario: Delete a revoked API key
    Given I have created an API key named "delete-revoked"
    And I have revoked the API key
    When I delete the API key
    Then I should not receive an error

  @api @apikey
  Scenario: Delete an expired API key
    Given I have created an API key named "delete-expired" with expiry "2s"
    And I wait 5 seconds
    When I delete the API key
    Then I should not receive an error
