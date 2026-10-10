---
title: Operating Fundament with funops
sidebar:
  label: funops (operator CLI)
  order: 4
---

`funops` is the command-line tool for Fundament operators. It does the
administrative work that no tenant API offers: creating organizations and
putting users in them. Signing in never creates an organization, so every new
user waits for an operator to run `funops`.

It talks to the PostgreSQL database directly, as a privileged user, so only
run it where you are allowed to change any tenant's data. The design is in
[FUN-8](/funs/fun-8).

## Get the binary

There is no prebuilt release. Build it from the repository:

```bash
go build -o funops ./funops/cmd/funops
```

## Connect to the database

`funops` reads one setting, `DATABASE_URL`, and connects as the `fun_operator`
database user (a superuser that bypasses row-level security). The Helm chart
stores that user's credentials in the `db-fun-operator` secret, next to the
`db-rw` service of the database cluster:

```bash
NS=fundament   # the namespace Fundament is installed in
kubectl port-forward -n "$NS" svc/db-rw 5432:5432 &
PASSWORD=$(kubectl get secret -n "$NS" db-fun-operator -o jsonpath='{.data.password}' | base64 -d)
export DATABASE_URL="postgresql://fun_operator:${PASSWORD}@localhost:5432/fundament"
```

On macOS, use `base64 -D`. Against the local development stack,
`just funops <args>` from the repository root does all of this for you.

## Commands

```
funops organization create <name>
funops organization list
funops organization delete <name>
funops organization member add <organization> <user-id|email> [--permission viewer|admin] [--create-user]
funops organization member list <organization>
funops organization member remove <organization> <user-id|email>
funops organization quota list <organization>
funops organization quota set-clusters <organization> <count>
funops organization quota set-nodes <organization> <region> <machine-type> <max-nodes>
funops user create <email> [--name <name>]
funops user list [--organization <name>] [--without-organization]
funops user delete <user-id|email>
```

Organizations are addressed by name, users by ID or email address. Every
command takes `-o json` for scripting and `--debug` for more logging. Data goes
to stdout and messages to stderr, so the output can be piped.

## Add a new user to an organization

Someone who has signed in but is in no organization yet shows up here:

```bash
funops user list --without-organization
```

Assign them, as a viewer or an admin:

```bash
funops organization member add acme-corp alice@acme-corp.com --permission admin
```

The membership is accepted right away, and the user's next token refresh picks
it up. If they hold an unanswered invitation to that organization, this accepts
it for them.

For someone who has not signed in yet, add `--create-user`. It registers the
address and assigns the membership in one go, and the account is linked when
they first sign in at that address. Without the flag an unknown address is an
error, so a typo cannot create an account nobody can claim. If a registration
was a typo after all, `funops user delete` removes it; it refuses accounts
somebody has signed in to.

## Set quotas

A new organization may create one cluster and no node pools: its cluster quota
is one and it has no node quota. Set what it may build:

```bash
funops organization quota set-clusters acme-corp 3
funops organization quota set-nodes acme-corp eu-west-1 n1-standard-2 20
```

The node quota is per region and machine type, named as the catalog offers
them, and caps the sum of the maximum sizes of all the organization's node
pools of that type, across its clusters. `funops organization quota list
acme-corp` shows every quota next to what is in use.

Lowering a quota below what is in use is allowed and changes nothing that
runs: it stops the next cluster, or the next new or larger pool, until there
is room again, and `quota list` shows the excess. Setting a node quota to 0
takes it away; the row stays and shows as 0. A deleted cluster and its pools
count until Gardener has torn the shoot down, as `organization delete` counts
them.

### After upgrading to quotas

The migration that introduces quotas gives every existing organization the
defaults of a new one: one cluster and no node quota. Nothing that runs is
touched, but from then on a second cluster, a new pool or a larger pool is
refused until the quotas are set. Review every organization right after the
migration:

```bash
funops organization list
funops organization quota list acme-corp
```

The list shows what is in use, including machine types the organization runs
without a quota row yet (quota 0, no "updated" time), so `set-clusters` and
`set-nodes` can give each organization at least the room it already uses.

## Remove users and organizations

`funops organization member remove` revokes a membership or a pending
invitation, and the user loses access through OpenFGA.

`funops organization delete` removes an organization, and revokes its
memberships and API keys with it. It refuses while the organization still has
clusters or publishes plugins; delete those first. A deleted cluster still
counts until Gardener confirms its shoot is gone. Nothing is thrown away: the
organization is marked deleted, disappears from `funops organization list`,
and its name can be used again, unless it ever had a cluster: its Gardener
project is named after the organization and is not removed yet, so
`funops organization create` refuses that name.
