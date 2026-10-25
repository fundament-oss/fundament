# A VM booted from a prebuilt qcow2, as the qemu provider runs it; KubeVirt and
# other image-based providers can boot the same image. The image is small; the
# root partition grows to the disk the provider gives it on first boot.
{ modulesPath, pkgs, lib, ... }:
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

  # TODO: remove this binfmt block once both are fixed:
  #   - ghcr.io/printeers/trek publishes arm64; then bump FROM in db/Dockerfile.
  #   - GARDENER_VERSION in test-env/mod.just is v1.140.0 or later (metrics-exporter 0.44.0).
  # x86_64 binaries run emulated on aarch64, as Docker Desktop runs them on a Mac:
  # ghcr.io/printeers/trek (the db image's base) is amd64-only, and Gardener's
  # metrics-exporter 0.43.0 ships an amd64 binary in its arm64 image.
  # The emulator is static so containers can run it (fixBinary), and non-PIE for
  # x86_64 only: nixpkgs' static-PIE qemu-x86_64 segfaults on start, and its
  # aarch64 targets fail to link.
  boot.binfmt.emulatedSystems = lib.optional pkgs.stdenv.hostPlatform.isAarch64 "x86_64-linux";
  boot.binfmt.registrations = lib.mkIf pkgs.stdenv.hostPlatform.isAarch64 {
    x86_64-linux = {
      interpreter = "${(pkgs.pkgsStatic.qemu-user.override { hostCpuTargets = [ "x86_64-linux-user" ]; }).overrideAttrs (o: {
        configureFlags = (o.configureFlags or [ ]) ++ [ "--disable-pie" ];
      })}/bin/qemu-x86_64";
      fixBinary = true;
    };
  };

  # The qemu driver logs the serial console into the instance directory.
  boot.kernelParams = [ "console=${if pkgs.stdenv.hostPlatform.isAarch64 then "ttyAMA0" else "ttyS0"},115200" ];
}
