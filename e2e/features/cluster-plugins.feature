# Runs only against a real Gardener, in the test-env VM: just test-env test @cluster-plugins
# Uses the baseline cluster (E2E_CLUSTER_NAME, default "e2e-base"; test-env sets it) and a
# plugin the system organization published (just test-env plugin-publish cert-manager).
@real-gardener @cluster-plugins @api
Feature: Plugins on a real cluster
  cluster-worker provisions the plugin machinery onto every ready cluster.
  A user installs a plugin with a PluginInstallation through the
  kube-api-proxy, as the console and the OpenTofu provider do; the plugin
  controller on the cluster runs it, and plugin-proxy serves its console.

  Background:
    Given I am authenticated as "alice@acme-corp.com"

  Scenario: The cluster runs the plugin controller
    Then the baseline cluster runs the plugin controller

  Scenario: Install a plugin, open its console, uninstall it
    Given the plugin "cert-manager" is not installed on the baseline cluster
    When I install the plugin "cert-manager" on the baseline cluster
    Then within 10 minutes the plugin "cert-manager" is running
    And the console of the plugin "cert-manager" is served through the plugin proxy
    When I uninstall the plugin "cert-manager"
    Then within 5 minutes the plugin "cert-manager" and its namespace are gone
