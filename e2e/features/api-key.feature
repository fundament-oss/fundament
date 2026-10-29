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

  @api @apikey @negative
  Scenario Outline: Refuse an invalid lifetime
    When I try to create an API key named "<name>" with expiry "<expiry>"
    Then I should receive an invalid argument error
    And the error message should contain "<message>"

    Examples:
      | name           | expiry  | message                                   |
      | lifetime-words | forever | must be a duration such as 300s, 5m or 1h |
      | lifetime-neg   | -1h     | must be a duration such as 300s, 5m or 1h |
      | lifetime-zero  | 0s      | must be positive                          |
