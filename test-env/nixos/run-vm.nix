# run-vm.sh with the qemu pinned by this flake. The qemu driver runs it inside the
# nixos/nix container, so the host needs neither qemu nor the kvm group.
{ writeShellApplication, qemu_kvm, xorriso, coreutils }:
writeShellApplication {
  name = "run-vm";
  runtimeInputs = [ qemu_kvm xorriso coreutils ];
  text = builtins.readFile ./run-vm.sh;
}
