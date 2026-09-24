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

Against the local development instance, run it as `just funops <args>` from the
repo root.
