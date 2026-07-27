---
title: API keys
sidebar:
  order: 9
---

API keys let scripts, CI pipelines, the [functl CLI](./functl.md) and the
[OpenTofu provider](./opentofu-provider.md) talk to the Fundament API without a
browser login. A key acts with the permissions of the identity that created it —
see [Members and roles](./members-and-roles.md).

## Creating a key

Go to **API keys** in the console and create a key with:

- **Name** — how you recognise it later (1–255 characters).
- **Expiry** — how long the key stays valid. A key can also be created without
  an expiry, in which case it is valid until you revoke it. Prefer an expiry.

The full token is shown **once, at creation time**. Copy it then; the console
only ever shows the first characters afterwards (the `fun_` prefix plus a few
identifying characters), so you cannot recover a lost token — create a new key
instead.

## Using a key

Set it in the environment:

```bash
export FUNDAMENT_API_KEY=fun_…
```

Both `functl` and the OpenTofu provider read `FUNDAMENT_API_KEY`. `functl` can
also store the key on disk with `functl auth login`, which writes it to
`~/.config/fundament/credentials`.

## Managing keys

The API keys list shows each key's name, prefix, creation time, last use and
expiry, so you can spot keys that are unused or about to expire.

Two operations are available:

- **Revoke** — invalidates the key immediately while keeping the record, so the
  key remains visible as revoked in the list.
- **Delete** — removes the key from your list.

Revoke a key as soon as you suspect it has leaked; revocation takes effect for
new requests right away.

## Good practice

- One key per consumer (per pipeline, per machine), so you can revoke a single
  one without disrupting anything else.
- Always set an expiry and rotate before it lapses.
- Never commit a key; use your CI system's secret storage.
- Check **last used** before deleting a key you no longer recognise.
