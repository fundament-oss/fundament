Feature: API Token Exchange
  As a system using Fundament
  I want to exchange API tokens for JWTs
  So that I can make authenticated API calls

  Background:
    Given I am authenticated as "alice@acme-corp.com"

  @api @token @smoke
  Scenario: Exchange valid API token for JWT
    Given I have created an API key named "exchange-test"
    And I have saved the full token
    When I call ExchangeToken with the API token
    Then I should receive a valid JWT
    And the JWT should expire in 900 seconds
    And the token type should be "Bearer"

  @api @token
  Scenario: Token exchange updates last used timestamp
    Given I have created an API key named "last-used-test"
    And I have saved the full token
    When I call ExchangeToken with the API token
    And I get the API key details
    Then the last used timestamp should be recent

  @api @token
  Scenario: Exchanged JWT can be used for API calls
    Given I have created an API key named "jwt-usage-test"
    And I have saved the full token
    When I call ExchangeToken with the API token
    And I use the exchanged JWT to list API keys
    Then the request should succeed

  @api @token @negative
  Scenario: Exchange token with missing Authorization header fails
    When I call ExchangeToken without an Authorization header
    Then I should receive an unauthenticated error

  @api @token @negative
  Scenario: Exchange invalid token format fails
    When I call ExchangeToken with token "invalid_token_format"
    Then I should receive an unauthenticated error

  @api @token @negative
  Scenario: Exchange token with wrong prefix fails
    When I call ExchangeToken with token "bad_abcdefghijklmnopqrstuvwxyz123456"
    Then I should receive an unauthenticated error

  @api @token @negative
  Scenario: Exchange token with invalid checksum fails
    When I call ExchangeToken with token "fun_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaabcdef"
    Then I should receive an unauthenticated error

  @api @token @negative
  Scenario: Exchange revoked API token fails
    Given I have created an API key named "revoked-exchange"
    And I have saved the full token
    And I have revoked the API key
    When I call ExchangeToken with the API token
    Then I should receive an unauthenticated error

  @api @token @negative
  Scenario: Exchange deleted API token fails
    Given I have created an API key named "deleted-exchange"
    And I have saved the full token
    And I have deleted the API key for exchange test
    When I call ExchangeToken with the API token
    Then I should receive an unauthenticated error

  @api @token @negative
  Scenario: Exchange non-existent token fails
    When I call ExchangeToken with a valid-format but non-existent token
    Then I should receive an unauthenticated error
