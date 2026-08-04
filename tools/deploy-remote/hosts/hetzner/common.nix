# Shared Hetzner Cloud VM config (hardware, boot, disk) — used by both the
# one-time-use test box (./default.nix) and the devbox (../hetzner-devbox).
# Role differences (ephemeral vs persistent state, dev tooling) live in the hosts.
{ pkgs, lib, ... }:
{
  imports = [
    ../../modules/baseline.nix
    ./disko.nix
  ];

  # Cloud networking: DHCP, no Wi-Fi.
  networking.useDHCP = lib.mkDefault true;

  # Minimal hardware for a Hetzner Cloud VM (virtio + common controllers).
  boot.initrd.availableKernelModules = [
    "virtio_pci" "virtio_scsi" "virtio_blk" "virtio_net" "ahci" "nvme" "sd_mod" "xhci_pci"
  ];
  boot.kernelModules = [ ];
  nixpkgs.hostPlatform = "x86_64-linux";

  # Hetzner Cloud x86_64 boots **legacy BIOS** (verified on a live box). Install
  # GRUB to the disk MBR via the EF02 bios_grub partition (see disko.nix), and also
  # emit an EFI image to the ESP as removable so the same config boots if a platform
  # ever comes up in UEFI mode. No NVRAM writes (cloud firmware).
  boot.loader.systemd-boot.enable = false;
  boot.loader.efi.canTouchEfiVariables = false;
  boot.loader.grub = {
    enable = true;
    efiSupport = true;
    efiInstallAsRemovable = true;
    devices = [ "/dev/sda" ]; # BIOS install target (MBR + EF02)
  };
}
