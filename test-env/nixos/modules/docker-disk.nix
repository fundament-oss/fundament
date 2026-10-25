# The large, throwaway state (container images, kind and k3d volumes, the login
# user's home with the toolchain and caches) lives on a dedicated disk that is
# formatted on every boot. A boot is therefore always a clean start.
#
# Providers supply the disk under a stable name: a virtio disk with serial
# `docker` (qemu, KubeVirt), or a partition labelled `docker` (Hetzner). A path
# in /var/lib/test-env/instance/docker-disk takes priority.
{ pkgs, ... }:
let
  user = "fundament";
  mnt = "/var/lib/docker-disk";
  candidates = [ "/dev/disk/by-id/virtio-docker" "/dev/disk/by-partlabel/docker" ];
in
{
  systemd.services.test-env-docker-disk = {
    description = "Format and mount the throwaway docker disk";
    wantedBy = [ "multi-user.target" ];
    requiredBy = [ "docker.service" ];
    before = [ "docker.service" "sshd.service" ];
    after = [ "local-fs.target" "test-env-instance-data.service" ];
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
    };
    path = [ pkgs.e2fsprogs pkgs.util-linux pkgs.coreutils pkgs.systemd ];
    script = ''
      set -euo pipefail
      udevadm settle
      dev=""
      if [ -s /var/lib/test-env/instance/docker-disk ]; then
        dev=$(cat /var/lib/test-env/instance/docker-disk)
      else
        for c in ${toString candidates}; do
          [ -e "$c" ] && dev=$c && break
        done
      fi
      [ -n "$dev" ] || { echo "no docker disk found (tried: ${toString candidates})" >&2; exit 1; }
      dev=$(readlink -f "$dev")
      # The root filesystem's device and every device under it (partition, disk,
      # also through LVM or LUKS). Without that list nothing is formatted.
      held=$(lsblk -nslpo NAME "$(findmnt -rno SOURCE /)") && [ -n "$held" ] \
        || { echo "refusing: cannot tell which devices hold the root filesystem" >&2; exit 1; }
      if grep -qxF "$dev" <<<"$held"; then
        echo "refusing: $dev holds the root filesystem" >&2
        exit 1
      fi

      mkfs.ext4 -q -F -L docker "$dev"
      mkdir -p ${mnt}
      mount "$dev" ${mnt}
      mkdir -p ${mnt}/docker ${mnt}/home /var/lib/docker /home/${user}
      mount --bind ${mnt}/docker /var/lib/docker
      mount --bind ${mnt}/home /home/${user}
      chown ${user}:users /home/${user}
      chmod 0700 /home/${user}
    '';
  };
}
