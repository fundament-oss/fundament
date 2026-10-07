# funops

Operator CLI tool for Fundament platform administration.

See [FUN-8](../docs/funs/FUN-8.adoc) for design details.

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
funops plugin list [--status <status>|all]
funops plugin approve <plugin|plugin-id> <version> --reviewer <user-id|email> [--feedback <text>]
funops plugin request-changes <plugin|plugin-id> <version> --reviewer <user-id|email> --feedback <text>
funops plugin reject <plugin|plugin-id> <version> --reviewer <user-id|email> --reason <reason> [--feedback <text>]
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
says otherwise; a new membership defaults to viewer. An invitation is not
membership: `user list` shows it in its own column, and neither
`--organization` nor `--without-organization` counts it.

`funops organization delete` soft-deletes the organization and revokes its
memberships and API keys. It refuses while the organization still has clusters,
including deleted ones whose shoot Gardener has not removed yet, or publishes
plugins. The name of a deleted organization that ever had a cluster cannot be
reused, because its Gardener project is named after it and is not removed yet.

The plugin commands are the operator path to the same review decisions the
marketplace admin console makes ([FUN-20](../docs/funs/FUN-20.adoc)): only a
pending version with an open submission round can be decided, approval is what
publishes it, and the decision is recorded on the round under the reviewer
named by `--reviewer` (a DCIM staff user). A plugin name that several
organizations use is refused; `--organization` narrows it down.

Against the local development instance, run it as `just funops <args>` from the
repo root. For other installations, see the
[operator page](../docs/developer/fundament/funops.md) in the documentation.
