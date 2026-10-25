# Per-instance data (the login key, an optional docker disk path) is not baked into
# the image, so one image serves every provider. It arrives on a cloud-init NoCloud
# volume labelled cidata (a seed ISO for qemu, cloudInitNoCloud on KubeVirt), or is
# written into /var/lib/test-env/instance before first boot (nixos-anywhere
# --extra-files on Hetzner). Only `ssh_authorized_keys` of the #cloud-config is read.
{ pkgs, ... }:
let
  dir = "/var/lib/test-env/instance";
in
{
  services.openssh.authorizedKeysFiles = [ "${dir}/authorized_keys.d/%u" ];

  systemd.services.test-env-instance-data = {
    description = "Read per-instance data from the cloud-init NoCloud volume";
    wantedBy = [ "multi-user.target" ];
    before = [ "sshd.service" "test-env-docker-disk.service" ];
    after = [ "local-fs.target" ];
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
    };
    path = [ pkgs.util-linux pkgs.yq-go pkgs.coreutils pkgs.systemd ];
    script = ''
      set -euo pipefail
      keys=${dir}/authorized_keys.d/fundament
      if [ -s "$keys" ]; then
        echo "instance data already present"
        exit 0
      fi

      udevadm settle
      dev=""
      for label in cidata CIDATA; do
        [ -e "/dev/disk/by-label/$label" ] && dev="/dev/disk/by-label/$label" && break
      done
      if [ -z "$dev" ]; then
        echo "no instance data: neither $keys nor a cidata volume" >&2
        exit 1
      fi

      mnt=$(mktemp -d)
      mount -o ro "$dev" "$mnt"
      trap 'umount "$mnt"; rmdir "$mnt"' EXIT
      mkdir -p ${dir}/authorized_keys.d
      yq -r '.ssh_authorized_keys[]' "$mnt/user-data" > "$keys"
      if [ -f "$mnt/docker-disk" ]; then
        cp "$mnt/docker-disk" ${dir}/docker-disk
      fi
      chmod 0644 "$keys"
    '';
  };
}
