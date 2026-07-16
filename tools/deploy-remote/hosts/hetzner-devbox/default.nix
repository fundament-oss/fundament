# Hetzner devbox: daily-cattle developer box. Same VM/disk layout as the test box
# (hosts/hetzner/common.nix) but persistent-state semantics:
#   - NO ephemeral-scratch: a mid-day reboot must not wipe running clusters;
#     clean state comes from destroying the box (`devbox down`), never from reboot.
#   - /var/lib/docker persists on the scratch partition across reboots (disko
#     formats it once at install; it dies with the box).
#   - /home/fundament is a Hetzner Cloud Volume (label "devbox-home") that
#     survives box destruction — repos, dotfiles, ~/.claude, caches.
{ pkgs, ... }:
{
  imports = [
    ./../hetzner/common.nix
    ../../modules/dev.nix
  ];

  networking.hostName = "fundament-devbox";

  # Docker state on the scratch partition — plain persistent mount, no boot wipe.
  # (disko creates+formats the partition at install time; see hosts/hetzner/disko.nix.)
  fileSystems."/var/lib/docker" = {
    device = "/dev/disk/by-partlabel/scratch";
    fsType = "ext4";
  };
  # Never let docker race the mount and write into an unmounted /var/lib/docker.
  systemd.services.docker.unitConfig.RequiresMountsFor = [ "/var/lib/docker" ];

  # The per-dev persistent volume, mounted by filesystem label so the config is
  # identical for every developer (the Hetzner by-id path differs per volume).
  # nofail: a box with no volume attached still boots and is SSH-reachable.
  fileSystems."/home/fundament" = {
    device = "/dev/disk/by-label/devbox-home";
    fsType = "ext4";
    options = [ "nofail" "x-systemd.device-timeout=60s" ];
  };

  # Label the attached Hetzner volume on first use. hetzner.sh creates the volume
  # pre-formatted (ext4) but the Hetzner API sets no filesystem label, and the
  # mount above addresses it by label. This oneshot finds the (single) attached
  # HC volume, labels a label-less ext4 (or formats a blank device as a fallback),
  # then starts the mount. Idempotent; a no-volume boot exits 0.
  systemd.services.devbox-home-setup = {
    description = "Label/adopt the devbox persistent home volume";
    wantedBy = [ "multi-user.target" ];
    after = [ "local-fs.target" ];
    path = [ pkgs.e2fsprogs pkgs.util-linux pkgs.systemd pkgs.coreutils pkgs.gnugrep ];
    serviceConfig = { Type = "oneshot"; RemainAfterExit = true; };
    script = ''
      set -euo pipefail
      dev=$(ls /dev/disk/by-id/scsi-0HC_Volume_* 2>/dev/null | head -1) || true
      if [ -z "''${dev:-}" ]; then
        echo "devbox-home-setup: no Hetzner volume attached; home stays on the root disk"
        exit 0
      fi
      fstype=$(blkid -o value -s TYPE "$dev" 2>/dev/null || true)
      label=$(blkid -o value -s LABEL "$dev" 2>/dev/null || true)
      if [ -z "$fstype" ]; then
        echo "devbox-home-setup: blank volume — formatting ext4 with label devbox-home"
        mkfs.ext4 -q -L devbox-home "$dev"
      elif [ "$fstype" = "ext4" ] && [ "$label" != "devbox-home" ]; then
        echo "devbox-home-setup: labeling existing ext4 volume"
        e2label "$dev" devbox-home
      fi
      udevadm trigger --settle "$dev" 2>/dev/null || udevadm trigger "$dev" || true
      systemctl start home-fundament.mount || true
    '';
  };

  # Don't accept SSH sessions before the home volume had its chance to mount —
  # otherwise vscode-server/dotfiles would land on the (empty) root-disk home and
  # be shadowed when the mount arrives. Ordering only: with no volume attached
  # (mount nofail) sshd still comes up.
  systemd.services.sshd.after = [ "devbox-home-setup.service" "home-fundament.mount" ];
}
