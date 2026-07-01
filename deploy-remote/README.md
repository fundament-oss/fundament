# deploy-remote — one-time-use fundament/Gardener box on Hetzner Cloud

Spin up a throwaway cloud box that runs the full fundament + Gardener local stack
(k3d, kind, MetalLB, Calico, local DNS) and drives a shoot to completion — then
destroy it. Deployed with **no local nix**: only `ssh`, `curl` and Docker are
needed (Docker you already run for k3d).

> Reset = **destroy + recreate** (`hetzner-down` then `hetzner-up`); a fresh box is
> already clean state.

## Layout

```
deploy-remote/
├── flake.nix                     nixosConfigurations.hetzner (built by nixos-anywhere)
├── justfile                      thin wrappers around hetzner.sh
├── hetzner.sh                    lifecycle: up · stack · certs · console · tunnel · ssh · status · down
├── box/                          scripts pushed to the box by `hetzner.sh stack`
│   ├── bootstrap.sh              clone fundament + apply patch + mise install
│   └── run-stack.sh              cluster-create → gardener-up → skaffold → drive a shoot to 100%
├── modules/
│   ├── baseline.nix             functional system: docker, nix-ld, resolved+gardener DNS, tools
│   └── ephemeral-scratch.nix    reformat the scratch partition before docker (reboot-to-clean)
├── hosts/hetzner/               default.nix · disko.nix (/dev/sda, EF02+ESP+scratch)
├── patches/                     k3d-gardener-coexist.patch (applied on the box)
├── secrets/hetzner.env.example  template; real secrets/hetzner.env (API token) is gitignored
└── cache/                       gitignored — fetched hcloud binary, mkcert CA, cloud-config
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
just hetzner-up        # create box + install NixOS (nixos-anywhere in docker); waits until ready
just hetzner-stack     # clone fundament + mise + gardener-up + skaffold + drive a shoot to 100%
just hetzner-console   # open Chrome/Chromium at the console UI (auto-tunnels :8443)
just hetzner-ssh       # log in (port 2022) to poke around
just hetzner-status    # list project servers — never forget a box is billing
just hetzner-down      # DESTROY the box — stops billing
```

`hetzner-up` needs `ssh` + `curl` + a running **Docker** daemon. It fetches a pinned
`hcloud`, registers your admin pubkey (`~/.ssh/id_rsa.pub`, override `ADMIN_PUBKEY=…`),
creates the box, then installs NixOS (~8–12 min). Override defaults via env, e.g.
`HZ_TYPE=ccx43 HZ_LOCATION=hel1 just hetzner-up` (try another location on
`resource_unavailable` — CX capacity varies by DC). Works on macOS and Linux.

`hetzner-stack` pushes `box/*.sh` + `patches/*` and runs bootstrap + the full cycle
(gardener-up ~10-15 min, shoot ~7). Re-runs cleanly.

## Certificates — trusted TLS

The stack's TLS is mkcert-signed. *Ignoring* the cert only gets you the browser UI;
`functl` (the shoot kubeconfig's exec auth) and `kubectl` can't skip verification. So
`hetzner-stack` **copies your machine's mkcert CA onto the box** (also `hetzner.sh
certs` standalone), so it signs `*.fundament.localhost` with a CA your OS already
trusts from local `mkcert -install` — no new trust entries, macOS and Linux alike
(`mkcert -CAROOT` resolves the right path). Then browser, `functl` and `kubectl` all
work with no `--insecure`. (No local mkcert CA → the box self-signs; `hetzner-console`
still works via `--ignore-certificate-errors`, but the kubeconfig path won't.)

## Reaching the console / clusters

The box's fundament is hardwired to the `https://*.fundament.localhost:8443` origin
(dex issuer, OIDC callbacks, API CORS), so it only works when reached at **exactly
that origin** — an SSH tunnel on local **8443**. One forwarded port serves every
`*.fundament.localhost` host (host-routed nginx; `*.localhost` → 127.0.0.1).

```sh
# a LOCAL k3d fundament owns 127.0.0.1:8443 — stop it first so the box can use that origin:
k3d cluster stop fundament
just hetzner-console                         # tunnel :8443 + open the console
kubectl --kubeconfig <shoot-kubeconfig> get nodes   # works with certs trusted, no --insecure
```

## Secrets & keys

| Material | Where at rest | How it reaches the box |
|---|---|---|
| Admin pubkey | `~/.ssh/id_rsa.pub` (public) + `modules/baseline.nix` | registered with hcloud for bootstrap |
| Hetzner Cloud API token | gitignored `secrets/hetzner.env` | `hetzner-*` → `HCLOUD_TOKEN` |
| Your mkcert CA | your OS mkcert CAROOT | `hetzner certs` / `stack` copies it to the box |

No private key material is baked into the flake (flake files land in world-readable
`/nix/store`). `cache/` holds the fetched hcloud binary + a copy of your mkcert CA —
gitignored; delete it when done.

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

- **Docker** — `hetzner-up` needs a running daemon (Docker Desktop, OrbStack, colima…).
  Select a non-default context: `DOCKER_CONTEXT=orbstack just hetzner-up`.
- **Legacy BIOS** — Hetzner Cloud x86_64 boots legacy BIOS; `hosts/hetzner` uses GRUB
  with a dual EF02 (BIOS) + ESP (UEFI-fallback) layout. Handled.
- **Reproducibility** — `nixpkgs` pinned via `flake.lock`; the on-box mise toolchain
  and Gardener (`v1.138.0`, cloned by gardener-up) are version-pinned.
