Feature: Node pool quota
  As an organization admin
  I want the node pool form to bound a pool by the organization's node quota
  So that I learn what fits before I save, and still hear why when it no
  longer fits by the time I do

  # A platform operator sets a node quota per organization, region and machine
  # type. The autoscale maxima of all the organization's pools of that machine
  # type, across its clusters, must stay within it; a database trigger checks
  # every create and every raise, and organization-api refuses with a message
  # that names the numbers. Lowering a maximum is always allowed.
  #
  # The fixtures give acme-corp 100 nodes per machine type
  # (db/testdata/049_0101-content.sql), but other clusters in acme-corp may
  # already use some, so the steps read the room from GetOrganizationQuotas
  # instead of assuming it. No cluster is seeded in region "local", so each
  # scenario creates one and removes it afterwards. Node pools are rows in the
  # database here: no Gardener is needed.

  Background:
    Given I am authenticated as "alice@acme-corp.com"
    And I have a cluster in region "local" with a "local-medium" node pool with maximum 2
    And I am logged in as "alice@acme-corp.com" with password "password"

  @api @node-pool-quota
  Scenario: The form refuses a maximum over the room before saving
    When I open the node pools of that cluster
    And I add a "local-medium" node pool with a maximum of the room left plus 1
    Then the new node pool says how much room is left
    When I save the node pools
    Then the form refuses the maximum before saving
    And the cluster has no node pool by that name

  # The form's bound is a hint: the room can run out between loading the form
  # and saving it, and then the API's refusal is what the user sees.
  @api @node-pool-quota
  Scenario: The API refuses a pool when the room ran out after the form loaded
    When I open the node pools of that cluster
    And I add a "local-medium" node pool with a maximum of the room left plus 0
    And another node pool takes 1 "local-medium" node in the meantime
    And I save the node pools
    Then I see a refusal that names the "local-medium" quota
    And the cluster has no node pool by that name

  @api @node-pool-quota
  Scenario: Lowering a maximum is saved with the organization at its quota
    Given the rest of the "local-medium" quota is taken
    When I open the node pools of that cluster
    And I lower the maximum of the existing node pool to 1
    And I save the node pools
    Then the existing node pool has maximum 1
