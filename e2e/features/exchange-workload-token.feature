@mock-verifier
Feature: Exchange Workload Token
  As a workload fundament provisions onto a cluster
  I want to exchange my projected ServiceAccount token for a WorkloadToken
  So that I can call fundament authenticated as that cluster's workload

  # Runs only where authn-api has SHOOT_VERIFIER_MODE=mock and the suite has
  # E2E_MOCK_SHOOT_VERIFIER=true (PR previews; see hooks.ts). There it
  # accepts HMAC tokens signed with the mock shoot secret for the seeded
  # acme-corp cluster, standing in for a TokenReview against a shoot. See
  # authn-api/pkg/shootverify/mock.go and db/testdata/001_0101-content.sql.
  # The subject is the shoot-side plugin-controller ServiceAccount (FUN-22).

  @api @workload @smoke
  Scenario: Exchange a projected token for the seeded cluster
    When I exchange a mock projected token for the seeded cluster
    Then I should receive a valid WorkloadToken
    And the workload token type should be "Bearer"
    And the workload token should expire in at most 900 seconds
    And the workload token audience should be "fundament-workload"
    And the workload token subject should be the seeded cluster
    And the workload token organization should be the seeded cluster's owner
    And the workload token workload should be "plugin-controller"

  @api @workload
  Scenario: The WorkloadToken never outlives the projected token
    When I exchange a mock projected token that expires in 300 seconds
    Then I should receive a valid WorkloadToken
    And the workload token should expire in at most 300 seconds

  @api @workload @negative
  Scenario: Exchange without a credential fails
    When I exchange a workload token without an Authorization header
    Then I should receive an unauthenticated error

  @api @workload @negative
  Scenario: A projected token for another audience is refused
    When I exchange a mock projected token with audience "https://kubernetes.default.svc"
    Then I should receive an unauthenticated error

  @api @workload @negative
  Scenario: A projected token of a ServiceAccount that is not allow-listed is refused
    When I exchange a mock projected token for subject "system:serviceaccount:default:default"
    Then I should receive an unauthenticated error

  @api @workload @negative
  Scenario: An unknown cluster is indistinguishable from a bad token
    When I exchange a mock projected token for an unknown cluster
    Then I should receive an unauthenticated error

  @api @workload @negative
  Scenario: A non-UUID cluster id is rejected by protovalidate
    When I exchange a mock projected token for cluster id "not-a-uuid"
    Then I should receive an invalid argument error

  @api @workload
  Scenario: A user token cannot be exchanged for a WorkloadToken (escalation wall)
    Given I am authenticated as "alice@acme-corp.com"
    When I exchange my user token as a workload credential
    Then I should receive an unauthenticated error

  @api @workload
  Scenario: A WorkloadToken is rejected as a user token (escalation wall)
    When I exchange a mock projected token for the seeded cluster
    And I use the workload token to call GetUserInfo
    Then I should receive an unauthenticated error

  @api @workload
  Scenario: A WorkloadToken cannot be exchanged again
    When I exchange a mock projected token for the seeded cluster
    And I exchange the workload token as a workload credential
    Then I should receive an unauthenticated error
