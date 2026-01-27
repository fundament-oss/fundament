Feature: User Authentication
  As a user of Fundament
  I want to log in to the platform
  So that I can manage my cloud infrastructure

  Background:
    Given I am on the login page

  @smoke @login
  Scenario: Successful login with valid credentials
    When I enter email "admin@example.com"
    And I enter password "password"
    And I click the sign in button
    Then I should be redirected to the dashboard
    And I should see the main navigation

  @login @negative
  Scenario: Failed login with invalid password
    When I enter email "admin@example.com"
    And I enter password "wrongpassword"
    And I click the sign in button
    Then I should see an error message
    And I should remain on the login page

  @login @negative
  Scenario: Failed login with non-existent user
    When I enter email "nonexistent@example.com"
    And I enter password "anypassword"
    And I click the sign in button
    Then I should see an error message
    And I should remain on the login page

  @login @validation
  Scenario: Login form validation - empty email
    When I enter password "somepassword"
    And I click the sign in button
    Then I should see a validation error containing "Email address is required"
    And I should remain on the login page

  @login @validation
  Scenario: Login form validation - invalid email format
    When I enter email "invalid-email"
    And I enter password "somepassword"
    And I click the sign in button
    Then I should see a validation error containing "valid email"
    And I should remain on the login page
