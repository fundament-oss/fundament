# A VM booted from a prebuilt qcow2, as the qemu provider runs it; KubeVirt and
# other image-based providers can boot the same image. The image is small; the
# root partition grows to the disk the provider gives it on first boot.
{ modulesPath, pkgs, ... }:
{
  imports = [
    ../../modules/base.nix
    "${modulesPath}/profiles/qemu-guest.nix"
    "${modulesPath}/virtualisation/disk-image.nix"
  ];

  networking.hostName = "fundament-test";

  # UEFI, the only boot path aarch64 has, so both architectures boot the same way.
  image.format = "qcow2";
  image.efiSupport = true;
  boot.loader.efi.canTouchEfiVariables = false;
  services.qemuGuest.enable = true;

  # The qemu driver logs the serial console into the instance directory.
  boot.kernelParams = [ "console=${if pkgs.stdenv.hostPlatform.isAarch64 then "ttyAMA0" else "ttyS0"},115200" ];
}
