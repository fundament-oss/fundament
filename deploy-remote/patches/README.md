# Local fundament-repo patches

Changes applied to a fundament checkout **on the box** to make the
fundament/Gardener smoke run on this NixOS host. Kept here so the fix is
reproducible without committing to the fundament repo proper. Apply from the
fundament repo root with `git apply <patch>`.

Each patch carries ONE concern, so `bootstrap.sh` can apply them independently:
when master gains one of the changes, that patch is detected as "already applied"
and the rest still land. A patch that fits neither state fails bootstrap hard
(upstream drift — regenerate it against current master).

## k3d-network.patch
Lets the fundament k3d cluster coexist with Gardener's local kind cluster.
- `Justfile`: `cluster-create` pre-creates a fixed `k3d-fundament` docker network
  (172.28.0.0/16) so k3d does not auto-grab 172.18.0.0/16, which Gardener's kind
  cluster reserves; `cluster-delete` removes it.
- `deploy/k3d/config.yaml`: pins k3d to that network.

## k3d-port-bind.patch
- `deploy/k3d/config.yaml`: publishes the ingress on `127.0.0.1:8443` (instead of
  `0.0.0.0:8443`) so it doesn't clash with Gardener. Already merged on some branches
  — then it no-ops as "already applied".

## Frontend toolchain (Node)
No patch needed. `mise.toml` pins `node`; on NixOS mise would otherwise build node from
source (V8 → ~50 min). Instead `box/{bootstrap,run-stack}.sh` set `MISE_NODE_COMPILE=0`,
so mise installs the **prebuilt** node, which runs via **nix-ld** (`programs.nix-ld.enable`
in `modules/baseline.nix`). Fast, and the full toolchain (node + npm: tools) is available.
