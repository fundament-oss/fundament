# funops

Operator CLI tool for Fundament platform administration.

See [FUN-8](../docs/funs/FUN-8.adoc) for design details.

## Commands

```
funops organization create <name>
funops organization list
funops organization delete <name>
funops organization member add <organization> <user-id|email> [--permission viewer|admin]
funops organization member list <organization>
funops organization member remove <organization> <user-id|email>
funops user create <email> [--name <name>]
funops user list [--organization <name>] [--without-organization]
```

Signing in never creates an organization. A new user shows up in
`funops user list --without-organization`; `funops organization member add`
puts them in one. Given an email address nobody has signed in with yet, it
registers the user first, so the membership is waiting when they do.

Against the local development instance, run it as `just funops <args>` from the
repo root.
