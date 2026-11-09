# Caches that outlive the VM: Go's build and module caches, mise's tools, the
# skaffold build cache and Gardener's registry caches. They hold only
# content-addressed data, so a fresh VM with a warm cache still starts from a
# clean state. Providers supply the disk as a virtio disk with serial `cache`;
# without one the caches live on the root disk and go with the VM.
{ pkgs, ... }:
let
  user = "fundament";
  dir = "/var/lib/test-env-cache";
in
{
  systemd.services.test-env-cache-disk = {
    description = "Mount the cache disk that outlives the VM";
    wantedBy = [ "multi-user.target" ];
    before = [ "sshd.service" ];
    after = [ "local-fs.target" ];
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
    };
    path = [ pkgs.e2fsprogs pkgs.util-linux pkgs.coreutils pkgs.systemd ];
    script = ''
      set -euo pipefail
      udevadm settle
      mkdir -p ${dir}
      dev=/dev/disk/by-id/virtio-cache
      if [ -e "$dev" ]; then
        blkid "$dev" >/dev/null || mkfs.ext4 -q -L test-env-cache "$dev"
        mount "$dev" ${dir}
      fi
      chown ${user}:users ${dir}
    '';
  };
}
