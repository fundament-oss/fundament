Feature: Authentication Session Management
  As a logged-in user of Fundament
  I want my authentication session to work correctly
  So that I can access protected resources and manage my session

  Background:
    Given I am logged in as "admin@example.com" with password "password"

  @auth @session
  Scenario: Auth cookie enables access to protected pages
    When I navigate to the dashboard
    Then I should see the dashboard content
    And the auth cookie should be set

  @auth @session
  Scenario: Auth cookie works for organization API calls
    When I navigate to a page that loads organization data
    Then the organization data should load successfully
    And I should not see an authentication error

  @auth @refresh
  Scenario: Token refresh maintains session
    Given my session is active
    When I trigger a token refresh
    Then I should remain authenticated
    And I should still have access to the dashboard

  @auth @logout
  Scenario: Logout clears authentication
    When I click the logout button
    Then I should be redirected to the login page
    And the auth cookie should be cleared
    And I should not be able to access the dashboard directly

  @auth @security
  Scenario: Tampered JWT payload is rejected
    Given I have a valid auth cookie
    When I modify the JWT payload to change the organization ID
    And I make an API request with the tampered token
    Then the API request should be rejected with an authentication error

  @auth @security
  Scenario: JWT with corrupted signature is rejected
    Given I have a valid auth cookie
    When I corrupt the JWT signature
    And I make an API request with the tampered token
    Then the API request should be rejected with an authentication error

  @auth @security
  Scenario: JWT with none algorithm is rejected
    Given I have a valid auth cookie
    When I modify the JWT to use the none algorithm
    And I make an API request with the tampered token
    Then the API request should be rejected with an authentication error

  @auth @security
  Scenario: JWT without signature is rejected
    Given I have a valid auth cookie
    When I remove the JWT signature
    And I make an API request with the tampered token
    Then the API request should be rejected with an authentication error

  @auth @security
  Scenario: Completely invalid JWT is rejected
    When I set the auth cookie to a random invalid value
    And I make an API request with the tampered token
    Then the API request should be rejected with an authentication error
