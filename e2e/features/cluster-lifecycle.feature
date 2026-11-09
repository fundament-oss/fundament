# Runs only against a real Gardener, in the test-env VM: just test-env test @cluster-lifecycle
# Creating and deleting a cluster takes 5 to 15 minutes each.
@real-gardener @cluster-lifecycle @api
Feature: Cluster lifecycle against a real Gardener
  As a user of Fundament
  I want to create a cluster, use it and delete it
  So that I can run workloads on clusters I manage myself

  Background:
    Given I am authenticated as "alice@acme-corp.com"

  Scenario: Create a cluster, use its kubeconfig, delete it
    When I create a cluster in region "local"
    Then within 20 minutes the cluster is running
    And its Gardener shoot was created successfully
    And the cluster's kubeconfig lists the namespace "kube-system"
    When I delete the cluster
    Then within 15 minutes the cluster is no longer listed
    And its Gardener shoot is gone
