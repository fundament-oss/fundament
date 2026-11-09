# Disk layout for a Hetzner Cloud box (single virtio-scsi disk = /dev/sda).
# nixos-anywhere runs disko to partition/format/mount before installing.
{
  disko.devices.disk.main = {
    type = "disk";
    device = "/dev/sda";
    content = {
      type = "gpt";
      partitions = {
        # BIOS boot partition: GRUB embeds core.img here for legacy-BIOS boot,
        # which is how Hetzner Cloud x86_64 actually boots (see default.nix).
        boot = {
          size = "1M";
          type = "EF02";
        };

        # ESP kept for the UEFI fallback (grub efiInstallAsRemovable writes here).
        ESP = {
          size = "1G";
          type = "EF00";
          content = {
            type = "filesystem";
            format = "vfat";
            mountpoint = "/boot";
            mountOptions = [ "umask=0077" ];
          };
        };

        # Root holds OS + nix store + the cloned fundament/gardener repos + mise
        # toolchain. 30G leaves the rest of the 160G (CX43) for the docker partition,
        # so the docker partition clears Gardener-local's 120Gi Docker disk requirement.
        root = {
          size = "30G";
          content = {
            type = "filesystem";
            format = "ext4";
            mountpoint = "/";
          };
        };

        # Throwaway container state: stable partlabel "docker", formatted on
        # every boot by modules/docker-disk.nix.
        docker = {
          label = "docker";
          size = "100%";
          content = {
            type = "filesystem";
            format = "ext4";
            # no mountpoint on purpose
          };
        };
      };
    };
  };
}
