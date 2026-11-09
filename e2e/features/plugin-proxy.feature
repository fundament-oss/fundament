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
  # "The e2e installation" is the mock plugin-proxy's fixed installation on the
  # seeded acme-cluster, where every asset and runtime call gets a canned reply,
  # or, against a real Gardener, a plugin installed on the baseline cluster,
  # whose pod serves the calls (support/fixtures.ts).

  Background:
    Given I am authenticated as "alice@acme-corp.com"

  @api @plugin-proxy @smoke
  Scenario: Asset bundle is served with the FUN-17 strict CSP
    Given the e2e installation is running
    When I GET the asset "/clusters/${CLUSTER_ID}/plugins/${INSTALLATION}/${VERSION}/console/certificates-list.html"
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
    Given I have a plugin token for the e2e installation
    When I send a GET to the installation route "/installations/00000000-0000-0000-0000-000000000099/runtime/api/ping" with the plugin token
    Then the response status should be 403

  # Path traversal in the install proxy is covered by the Go unit tests in
  # plugin-proxy/pkg/installproxy/handler_test.go ("traversal in tail",
  # "traversal in install id"). An e2e scenario cannot exercise this guard
  # because Node's fetch normalises `..` per WHATWG before the request is sent.

  @api @plugin-proxy @smoke
  Scenario: Authorized runtime call is forwarded to the installation's runtime
    Given I have a plugin token for the e2e installation
    When I send a GET to the installation route "/installations/${INSTALLATION_ID}/runtime/livez" with the plugin token
    Then the response status should be 200
    And the body is the installation runtime's liveness reply
