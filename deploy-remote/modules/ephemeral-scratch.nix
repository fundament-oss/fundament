# Shared clean-state mechanism. Reformats the dedicated "scratch" partition on
# every boot so Docker / kind / k3d always start clean. Together with the reboot
# clearing kernel residue (network namespaces, iptables/nftables, loop devices)
# this gives reproducible clean state per boot.
#
# Requires a GPT partition labelled "scratch" — see each host's disko.nix.
{ pkgs, ... }:
let
  user = "thom";
  scratchDevice = "/dev/disk/by-partlabel/scratch";
  dockerDir = "/var/lib/docker";
in
{
  systemd.services.scratch-wipe = {
    description = "Blank the ephemeral container-state scratch volume";
    wantedBy = [ "multi-user.target" ];
    # Docker MUST NOT start before this finishes; a failed wipe also blocks Docker.
    requiredBy = [ "docker.service" ];
    before = [ "docker.service" ];
    after = [ "local-fs.target" ];

    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
    };

    path = [ pkgs.e2fsprogs pkgs.util-linux pkgs.coreutils ];

    script = ''
      set -euo pipefail

      if mountpoint -q "${dockerDir}"; then
        umount -R "${dockerDir}" || true
      fi

      mkfs.ext4 -q -F -L scratch "${scratchDevice}"

      mkdir -p "${dockerDir}"
      mount "${scratchDevice}" "${dockerDir}"

      rm -rf "/home/${user}/.kube" \
             "/home/${user}/.config/k3d" \
             "/home/${user}/.config/kind" || true
    '';
  };
}
