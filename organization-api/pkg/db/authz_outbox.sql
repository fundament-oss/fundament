-- name: AuthzOutboxPendingForCluster :one
-- Whether a decision on the cluster may still change: a row for the cluster or
-- the caller's organization membership has yet to reach OpenFGA.
SELECT EXISTS (
  SELECT 1
  FROM authz.outbox
  WHERE authz.outbox.processed IS NULL
    AND authz.outbox.status IN ('pending', 'retrying')
    AND (
      authz.outbox.cluster_id = @cluster_id::uuid
      OR authz.outbox.organization_user_id IN (
        SELECT tenant.organizations_users.id
        FROM tenant.organizations_users
        JOIN tenant.clusters ON tenant.clusters.organization_id = tenant.organizations_users.organization_id
        WHERE tenant.clusters.id = @cluster_id
          AND tenant.organizations_users.user_id = @user_id
          AND tenant.organizations_users.deleted IS NULL
      )
    )
);

-- name: AuthzOutboxPendingForProject :one
-- Whether a decision on the project may still change: a row for the project, its
-- cluster, or the caller's project or organization membership has yet to reach OpenFGA.
SELECT EXISTS (
  SELECT 1
  FROM authz.outbox
  WHERE authz.outbox.processed IS NULL
    AND authz.outbox.status IN ('pending', 'retrying')
    AND (
      authz.outbox.project_id = @project_id::uuid
      OR authz.outbox.cluster_id IN (
        SELECT tenant.projects.cluster_id
        FROM tenant.projects
        WHERE tenant.projects.id = @project_id
      )
      OR authz.outbox.project_member_id IN (
        SELECT tenant.project_members.id
        FROM tenant.project_members
        WHERE tenant.project_members.project_id = @project_id
          AND tenant.project_members.user_id = @user_id
          AND tenant.project_members.deleted IS NULL
      )
      OR authz.outbox.organization_user_id IN (
        SELECT tenant.organizations_users.id
        FROM tenant.organizations_users
        JOIN tenant.clusters ON tenant.clusters.organization_id = tenant.organizations_users.organization_id
        JOIN tenant.projects ON tenant.projects.cluster_id = tenant.clusters.id
        WHERE tenant.projects.id = @project_id
          AND tenant.organizations_users.user_id = @user_id
          AND tenant.organizations_users.deleted IS NULL
      )
    )
);
