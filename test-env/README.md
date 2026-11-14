# test-env

fundament with a real local Gardener, in a VM, for e2e scenarios and real clusters.
How to use it: [docs/developer/fundament/test-env.md](../docs/developer/fundament/test-env.md).

```
test-env/
├── mod.just              just test-env <verb>; host or VM decided by /etc/fundament-test-env
├── host/
│   ├── cli.sh            host verbs: up, sync, redeploy, test, attach, run, collect, ssh, tunnel, down
│   ├── lib.sh            everything after "an SSH-reachable NixOS host": seed, CA, source, bundles
│   └── providers/        one driver per provider: create, address, destroy
│       ├── qemu.sh       local VM on Linux; qemu runs in the nixos/nix container with /dev/kvm
│       ├── macos.sh      local VM on Apple silicon; Homebrew's qemu under Apple's hypervisor
│       └── hetzner.sh    Hetzner Cloud server, NixOS installed with nixos-anywhere
├── box/                  runs in the VM: up.sh, test.sh, session.sh, collect.sh, redeploy.sh, plugin-publish.sh
└── nixos/
    ├── flake.nix         nixosConfigurations.{vm,hetzner}; packages.<system>.{vm-image,run-vm}
    ├── run-vm.sh         boots one instance: KVM on Linux, Apple's hypervisor on macOS
    ├── run-vm.nix        run-vm.sh with the flake's qemu
    ├── modules/          base, network, instance-data, docker-disk, memlog
    └── hosts/            vm (qcow2 image), hetzner (disko)
```

A provider driver brings up an SSH-reachable NixOS host with the modules from
`nixos/` and a disk for the throwaway state. Everything after that is the same
for every provider:

- **Per-instance data** (the login key) arrives on a cloud-init NoCloud volume
  labelled `cidata`, or in `/var/lib/test-env/instance` before first boot. One
  image serves every provider.
- **The docker disk** is a virtio disk with serial `docker`, or a partition
  labelled `docker`. It is formatted on every boot and holds `/var/lib/docker`
  and the login user's home, so a boot is a clean start.

A new provider (KubeVirt, a Fundament VM) adds a file in `host/providers/` with
`provider_create`, `provider_status` and `provider_destroy`, and either boots the
image (`nix_build vm-image`) or installs the flake with nixos-anywhere.

The image is built for the host's architecture, x86_64 or aarch64, and boots with
UEFI on both. On macOS, Docker builds it in an arm64 nixos/nix container, and
Homebrew's qemu (`brew install qemu xorriso`) boots it under Apple's hypervisor
(HVF) with the same `nixos/run-vm.sh` as on Linux. Apple's Virtualization.framework
(vfkit, Lima) would need a second boot path and raw disks instead of qcow2 overlays.
