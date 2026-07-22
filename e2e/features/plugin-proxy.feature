Feature: Plugin Proxy external listener
  As the FUN-17 plugin sandbox
  I want plugin-proxy's external listener to serve plugin asset bundles
  and proxy installation-bound RPCs behind a PluginToken
  So that an iframe-hosted plugin can load its JS and call kube without
  leaving its origin

  # "external listener" = port 8080, the only listener exposed via the
  # ingress. It carries two route groups:
  #   - /clusters/{id}/plugins/...  — console UserToken cookie required,
  #                           OpenFGA can_view(user, cluster), FUN-17 strict CSP
  #   - /installations/...  — PluginToken required (aud=fundament-plugin),
  #                           installation_id must match the URL,
  #                           OpenFGA can_view(user, cluster) re-checked
  # The internal listener (port 8081, PluginInstallationService RPC) is
  # not exercised here.
  #
  # The mock plugin-proxy (PLUGIN_PROXY_MODE=mock) pins every asset lookup to
  # MockClusterID and serves a canned HTML body; the installation proxy returns
  # a canned 200 "mock backend". The seeded fixtures (cluster + installation)
  # come from db/testdata/001_0101-content.sql and plugin-proxy/pkg/service/mock.go.

  Background:
    Given I am authenticated as "alice@acme-corp.com"

  @api @plugin-proxy @smoke
  Scenario: Asset bundle is served with the FUN-17 strict CSP
    When I GET the asset "/clusters/${CLUSTER_ID}/plugins/cert-manager/v1.17.2/console/index.html"
    Then the response status should be 200
    And the "Content-Type" header should start with "text/html"
    And the "Cache-Control" header should contain "private"
    And the "Cache-Control" header should contain "immutable"
    And the "Content-Security-Policy" header should contain "default-src 'self'"
    And the "Content-Security-Policy" header should contain "script-src 'self'"
    And the "Content-Security-Policy" header should contain "frame-ancestors ${CONSOLE_URL}"
    And the "Content-Security-Policy" header should contain "base-uri 'none'"
    And the "Content-Security-Policy" header should contain "object-src 'none'"
    And the "Content-Security-Policy" header should not contain "unsafe-inline"
    And the "X-Content-Type-Options" header should be "nosniff"

  @api @plugin-proxy @negative
  Scenario Outline: Asset paths with traversal or missing segments are rejected
    When I GET the asset "<path>"
    Then the response status should not be in the 2xx range

    Examples:
      | path                                                       |
      | /clusters/${CLUSTER_ID}/plugins/cert-manager/v1.17.2/console/../etc/passwd |
      | /clusters/${CLUSTER_ID}/plugins/cert-manager/v1.17.2/console/              |

  @api @plugin-proxy @negative
  Scenario: Installation route without a token is unauthorized
    When I send a GET to the installation route "/installations/00000000-0000-0000-0000-000000000001/runtime/api/ping" with no token
    Then the response status should be 401

  @api @plugin-proxy @negative
  Scenario: Installation-id in the URL must match the token claim
    Given I have a plugin token for the seeded installation
    When I send a GET to the installation route "/installations/00000000-0000-0000-0000-000000000099/runtime/api/ping" with the plugin token
    Then the response status should be 403

  # Path traversal in the install proxy is covered by the Go unit tests in
  # plugin-proxy/pkg/installproxy/handler_test.go ("traversal in tail",
  # "traversal in install id"). An e2e scenario cannot exercise this guard
  # because Node's fetch normalises `..` per WHATWG before the request is sent.

  @api @plugin-proxy @smoke
  Scenario: Authorized runtime call is forwarded to the mock backend
    Given I have a plugin token for the seeded installation
    When I send a GET to the installation route "/installations/00000000-0000-0000-0000-000000000001/runtime/api/ping" with the plugin token
    Then the response status should be 200
    And the response body should equal "mock backend"
