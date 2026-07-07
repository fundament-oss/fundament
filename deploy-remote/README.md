# deploy-remote — one-time-use fundament/Gardener box on Hetzner Cloud

Spin up a throwaway cloud box that runs the full fundament + Gardener local stack
(k3d, kind, MetalLB, Calico, local DNS) and drives a shoot to completion — then
destroy it. Deployed with **no local nix**: only `ssh`, `curl` and Docker are
needed (Docker you already run for k3d).

> Reset = **destroy + recreate** (`down` then `up`); a fresh box is
> already clean state.

## Layout

```
deploy-remote/
├── flake.nix                     nixosConfigurations.hetzner (built by nixos-anywhere)
├── justfile                      thin wrappers around hetzner.sh
├── hetzner.sh                    lifecycle: up · stack · certs · tunnel · ssh · status · down
├── box/                          scripts pushed to the box by `hetzner.sh stack`
│   ├── bootstrap.sh              clone fundament + checkout deployed ref + mise install
│   └── run-stack.sh              cluster-create → gardener-up → skaffold → drive a shoot to 100%
├── modules/
│   ├── baseline.nix             functional system: docker, nix-ld, resolved+gardener DNS, tools
│   └── ephemeral-scratch.nix    reformat the scratch partition before docker (reboot-to-clean)
├── hosts/hetzner/               default.nix · disko.nix (/dev/sda, EF02+ESP+scratch)
├── secrets/hetzner.env.example  template; real secrets/hetzner.env (API token) is gitignored
└── cache/                       gitignored — fetched hcloud binary, install key, box CA cert
```

## Deploy model — `nixos-anywhere` in a container (no local nix)

`hetzner.sh` creates the box with a pinned static `hcloud` binary, then runs
`nixos-anywhere` inside a throwaway `nixos/nix` **Docker container**, doing a clean
disko install. Why the container: `nixos-anywhere` needs to run as a *trusted* nix
user for `--build-on-remote` (the box builds its own x86_64 closure — your machine
never cross-builds). On a stock nix install the login user often isn't in
`trusted-users`, which silently breaks that; **in a container root is trusted by
default**, so it just works. (`nixos-infect` was tried and rejected — it left a
broken hybrid `/etc` with no network.)

The box's `/var/lib/docker` is a dedicated ext4 **scratch** partition reformatted on
every boot (`ephemeral-scratch.nix`), so a reboot returns to clean container state.

## Workflow

```sh
cd deploy-remote
cp secrets/hetzner.env.example secrets/hetzner.env   # paste a Read&Write API token
just up        # create box + install NixOS (nixos-anywhere in docker); waits until ready
just stack     # deploy fundament + Gardener, run a shoot, trust the box CA; prints the console URL
just tunnel    # SSH tunnel :8443 — then open the printed URL in your normal browser
just ssh       # log in (port 2022) to poke around
just status    # list project servers — never forget a box is billing
just down      # DESTROY the box — stops billing (also untrusts the box CA)
```

Every recipe is also exposed at the repo root: `just deploy-remote up`, etc.

**Time to ready:** ~35–45 min end-to-end from `just up` to a reachable
console — `up` ~8–10 min (box create + NixOS install), `stack`
~25–35 min (bootstrap + toolchain ~3, gardener-up ~10–15, fundament deploy ~5,
shoot to `Create Succeeded` ~7).

`up` needs `ssh` + `curl` + a running **Docker** daemon. It fetches a pinned
`hcloud`, generates a **throwaway install key** per deploy (registered with hcloud for
the root install phase — your own key, which may be passphrase-protected, never enters
the install container), bakes your admin pubkey in as the box **login** key (prefers
`~/.ssh/id_ed25519.pub`, falls back to `id_rsa.pub`; override `ADMIN_PUBKEY=…`), creates
the box, then installs NixOS (~8–12 min). Override defaults via env, e.g. `HZ_TYPE=ccx43
HZ_LOCATION=hel1 just up` (try another location on `resource_unavailable`).
Works on macOS/Linux.

`stack` pushes `box/*.sh` and runs bootstrap + the full cycle
(gardener-up ~10-15 min, shoot ~7), then trusts the box's CA (see below) and prints
how to reach the UIs. Re-runs cleanly.

> **The box runs your branch as PUSHED to origin — not your local working tree.**
> Bootstrap clones the repo from GitHub and checks out the branch you ran
> `stack` from; uncommitted or unpushed changes are NOT on the box.
> Push first, then run `stack` (re-runs re-fetch the branch).

## Certificates — ephemeral per-box CA

The stack's TLS is mkcert-signed. *Ignoring* the cert only gets you the browser UI;
`functl` (the shoot kubeconfig's exec auth) and `kubectl` can't skip verification — so
we need a genuinely trusted cert. Each box generates its **own mkcert CA** on deploy;
`stack` fetches that CA and runs **`mkcert -install`** locally (system + browser
NSS, macOS/Linux), so browser, `functl` and `kubectl` trust the box with no `--insecure`.
**Your real mkcert CA never touches the box** — only the box's throwaway CA is fetched,
and `down` runs `mkcert -uninstall` + deletes the local copy, so nothing lingers.
(`certs` re-trusts it, e.g. from a second machine.)

## Reaching the console / clusters

The box's fundament is hardwired to the `https://*.fundament.localhost:8443` origin
(dex issuer, OIDC callbacks, API CORS), so it only works when reached at **exactly
that origin** — an SSH tunnel on local **8443**. One forwarded port serves every
`*.fundament.localhost` host (host-routed nginx; `*.localhost` → 127.0.0.1).

```sh
# a LOCAL k3d fundament owns 127.0.0.1:8443 — stop it first so the box can use that origin:
k3d cluster stop fundament
just tunnel                                 # opens the tunnel + prints the URL
open https://console.fundament.localhost:8443       # in your normal browser (cert trusted)
kubectl --kubeconfig <shoot-kubeconfig> get nodes   # works with certs trusted, no --insecure
```

## Secrets & keys

| Material | Where at rest | How it reaches the box |
|---|---|---|
| Admin pubkey (login) | `~/.ssh/id_ed25519.pub` (or `admin_pubkey` in `secrets/hetzner.env`) | materialized per-deploy into gitignored `cache/admin-keys.nix`, imported by `modules/baseline.nix` |
| Install key (throwaway) | gitignored `cache/install-key` | generated + registered with hcloud per `up`; the only private key mounted into the nixos-anywhere container |
| Hetzner Cloud API token | gitignored `secrets/hetzner.env` | `hetzner.sh` → `HCLOUD_TOKEN` |
| Box's ephemeral CA (cert only) | fetched to gitignored `cache/box-ca/` | generated on the box; `mkcert -install`ed locally, `-uninstall`ed on `down` — the CA **private key** stays on the box |

No admin key is hardcoded in the flake — `hetzner.sh up` writes the operator's own
pubkey to gitignored `cache/admin-keys.nix`, which `baseline.nix` imports (empty if
absent). No private key material is baked into the flake (flake files land in
world-readable `/nix/store`; the flake source is staged sanitized — no `secrets/`,
no keys). `cache/` holds the fetched hcloud binary, the generated admin-keys file,
the throwaway install key, and the box CA cert — gitignored; delete it when done.

## Sizing & cost

Gardener-local needs **≥8 vCPU / 8Gi RAM / 120Gi Docker disk**
([docs](https://gardener.cloud/docs/gardener/deployment/getting_started_locally/)), and
fundament runs on top — but in practice **16 GB OOMs** (gardener runs several
apiservers — virtual garden + seed + shoot control plane — and the kernel oom-killed
`kube-apiserver` on a cx43). Default is **`HZ_TYPE=cx53`** (16 vCPU / 32 GB / 320 GB,
shared Intel, EU-only, ~**€0.036/h**), the cost-optimal CX line; `ccx43` (64 GB) only
if 32 GB isn't enough. Billing is hourly — tear down promptly. (ARM CAX excluded: the
flake is x86_64.)

## Caveats

- **Docker** — `up` needs a running daemon (Docker Desktop, OrbStack, colima…).
  Select a non-default context: `DOCKER_CONTEXT=orbstack just up`.
- **Legacy BIOS** — Hetzner Cloud x86_64 boots legacy BIOS; `hosts/hetzner` uses GRUB
  with a dual EF02 (BIOS) + ESP (UEFI-fallback) layout. Handled.
- **Reproducibility** — `nixpkgs` pinned via `flake.lock`; the on-box mise toolchain
  and Gardener (`v1.138.0`, cloned by gardener-up) are version-pinned.
