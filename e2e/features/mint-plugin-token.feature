Feature: Mint Plugin Token
  As a user of Fundament
  I want to mint a short-lived PluginToken bound to a plugin installation
  So that the plugin can act on my behalf against kube-api-proxy and plugin-proxy

  # The mock plugin-proxy (PLUGIN_PROXY_MODE=mock) serves a single
  # PluginInstallation whose UID is fixed; the cluster id is the one acme-corp
  # owns in the test-data seed. See db/testdata/001_0101-content.sql and
  # plugin-proxy/pkg/service/mock.go (MockClusterID, MockInstallationUID).

  Background:
    Given I am authenticated as "alice@acme-corp.com"

  @api @mint @smoke
  Scenario: Mint a plugin token for the seeded installation
    When I mint a plugin token for the seeded cluster and installation
    Then I should receive a valid JWT for plugin use
    And the JWT should expire in 900 seconds
    And the token type should be "Bearer"
    And the plugin token audience should be "fundament-plugin"
    And the plugin token subject should be the authenticated user
    And the plugin token should bind the cluster and installation
    And the plugin token should carry the plugin name "cert-manager"

  @api @mint @negative
  Scenario: Mint without authentication fails
    When I mint a plugin token without an Authorization header
    Then I should receive an unauthenticated error

  @api @mint @negative
  Scenario: Mint with a non-UUID cluster id is rejected by protovalidate
    When I mint a plugin token with cluster id "not-a-uuid"
    Then I should receive an invalid argument error

  @api @mint @negative
  Scenario: Mint with a non-UUID installation id is rejected by protovalidate
    When I mint a plugin token with installation id "not-a-uuid"
    Then I should receive an invalid argument error

  @api @mint @negative
  Scenario: Mint for a cluster the user cannot view collapses to NotFound
    When I mint a plugin token for an unknown cluster
    Then I should receive a not found error

  @api @mint
  Scenario: Minted plugin token is rejected as a user token (escalation wall)
    When I mint a plugin token for the seeded cluster and installation
    And I use the minted plugin token to call GetUserInfo
    Then I should receive an unauthenticated error
