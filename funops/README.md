# funops

Operator CLI tool for Fundament platform administration.

See [FUN-8](../docs/funs/FUN-8.adoc) for design details.

## Commands

```
funops cluster sync <cluster-id>
funops cluster sync --all
funops organization create <name> [--node-quota <region>/<machine-type>=<max> ...]
funops organization list
funops organization delete <name>
funops organization member add <organization> <user-id|email> [--permission admin] [--create-user]
funops organization member list <organization>
funops organization member remove <organization> <user-id|email>
funops organization quota get-clusters <organization>
funops organization quota set-clusters <organization> <count>
funops organization quota list-nodes <organization>
funops organization quota set-nodes <organization> <region> <machine-type> <max-nodes>
funops user create <email> [--name <name>]
funops user list [--organization <name>] [--without-organization]
funops user delete <user-id|email>
funops plugin list [--status <status|all>]
funops plugin approve <plugin-id|plugin> <version> --reviewer <user-id|email> [--organization <name>] [--feedback <text>]
funops plugin request-changes <plugin-id|plugin> <version> --reviewer <user-id|email> [--organization <name>] --feedback <text>
funops plugin reject <plugin-id|plugin> <version> --reviewer <user-id|email> [--organization <name>] --reason <incomplete_metadata|duplicate|security_concerns|naming_guidelines|out_of_scope|other> [--feedback <text>]
```

`funops cluster sync` queues a sync of a cluster to Gardener: the cluster-worker
writes the cluster's Shoot again, as it does after a change to the cluster, and
Gardener reconciles the Shoot when its spec changed. Run it with `--all` after
deploying a release that changes what the worker writes to every Shoot. It
prints the IDs of the clusters it queued; deleted clusters and clusters that
already have a sync waiting are left out. With a cluster ID, queuing nothing is
an error.

A sync writes the Kubernetes version and the worker pools from the database,
with the machine image version from the cluster-worker's
`GARDENER_MACHINE_IMAGE_VERSION` (and, on the local provider, the machine type
from `GARDENER_DEFAULT_MACHINE_TYPE`). A Shoot that differs from them, for
example after a Gardener maintenance update, is set back to those values, so
compare the two before running `--all`. Per cluster, the database:

```
psql "$DATABASE_URL" -c "SELECT tenant.clusters.id, tenant.clusters.kubernetes_version, tenant.node_pools.machine_type, tenant.node_pools.autoscale_min, tenant.node_pools.autoscale_max FROM tenant.clusters JOIN tenant.node_pools ON tenant.node_pools.cluster_id = tenant.clusters.id AND tenant.node_pools.deleted IS NULL WHERE tenant.clusters.deleted IS NULL ORDER BY tenant.clusters.id"
```

and the Shoots in the garden:

```
kubectl get shoots -A -l fundament.io/cluster-id -o custom-columns='CLUSTER:.metadata.labels.fundament\.io/cluster-id,VERSION:.spec.kubernetes.version,TYPE:.spec.provider.workers[*].machine.type,MIN:.spec.provider.workers[*].minimum,MAX:.spec.provider.workers[*].maximum,IMAGE:.spec.provider.workers[*].machine.image.version'
```

Signing in never creates an organization. A new user shows up in
`funops user list --without-organization`; `funops organization member add`
puts them in one. For someone who has not signed in yet, pass `--create-user`
with their email address: the user is registered and assigned in one go, so
the membership is waiting when they do. Without the flag an unknown address is
an error, so a typo cannot turn into an account nobody can claim; a
registration that was a typo after all goes away with `funops user delete`,
which only deletes users nobody has signed in with.

Adding someone who holds an unanswered invitation accepts it on their behalf.
The invitation keeps the permission it was sent with unless `--permission`
says otherwise; a new membership defaults to admin (the viewer role is
temporarily disabled, so `--permission` only takes `admin`). An invitation is not
membership: `user list` shows it in its own column, and neither
`--organization` nor `--without-organization` counts it.

`funops organization quota` sets what an organization may build. A new one
may have one cluster, and node pools within the starting node quotas given
with `organization create --node-quota`, if any (a cluster runs only once it has
a pool). A machine type without a quota row has a quota of 0 and allows no pool,
so `set-nodes` comes before a pool of a type without a starting quota, and
`set-clusters` before a second cluster.
The node quota is per region
and machine type and caps the sum of the maximum sizes of the organization's
node pools of that type. A quota only stops something new: lowering it below
what is in use is allowed, leaves the existing clusters and pools alone, and
`quota get-clusters` and `quota list-nodes` show the excess.

Upgrading an installation to the release that introduces quotas puts every
existing organization at one cluster and no node quota, while its clusters and
pools keep running. Review each one right after the migration:
`quota get-clusters` and `quota list-nodes` show what is in use, the latter
including machine types that have no quota row yet, and
`set-clusters` and `set-nodes` give it room before anyone adds a cluster or a
pool.

`funops organization delete` soft-deletes the organization and revokes its
memberships and API keys. It refuses while the organization still has clusters,
including deleted ones whose shoot Gardener has not removed yet, or publishes
plugins. The name of a deleted organization that ever had a cluster cannot be
reused, because its Gardener project is named after it and is not removed yet.

The plugin commands are the operator path to the same review decisions the
marketplace admin console makes ([FUN-20](../docs/funs/FUN-20.adoc)): only a
pending version with an open submission round can be decided, approval is what
publishes it, and the decision is recorded on the round under the reviewer
named by `--reviewer` (a DCIM staff user). A plugin is addressed by the ID
`funops plugin list` shows, or by name with `--organization` — plugin names
are unique only within an organization.

Against the local development instance, run it as `just funops <args>` from the
repo root. For other installations, see the
[operator page](../docs/developer/fundament/funops.md) in the documentation.
