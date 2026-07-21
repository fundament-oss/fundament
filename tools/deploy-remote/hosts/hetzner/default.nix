# Hetzner: a one-time-use cloud box (cattle) — cloud networking, no preserved identity.
# Installed by nixos-anywhere run inside a throwaway nix container (see hetzner.sh)
# — no local nix required; the box itself builds the closure (--build-on-remote).
{ ... }:
{
  imports = [
    ./common.nix
    ../../modules/ephemeral-scratch.nix # reboot-to-clean; drop for a pure throwaway
  ];

  networking.hostName = "fundament-test";
}
