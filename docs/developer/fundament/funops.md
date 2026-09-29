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

## Remove users and organizations

`funops organization member remove` revokes a membership or a pending
invitation, and the user loses access through OpenFGA.

`funops organization delete` removes an organization, and revokes its
memberships and API keys with it. It refuses while the organization still has
clusters or publishes plugins; delete those first. Nothing is thrown away: the
organization is marked deleted, disappears from `funops organization list`,
and its name can be used again.
