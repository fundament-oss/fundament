{
  description = "fundament test environment: NixOS for a local VM and a Hetzner Cloud box";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    disko = {
      url = "github:nix-community/disko";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs =
    { self, nixpkgs, disko, ... }:
    let
      # The guest runs the host's architecture: x86_64 under KVM on Linux,
      # aarch64 under KVM or Apple's hypervisor. Nix builds for the system it runs
      # on, so `nix build .#vm-image` gives the matching image.
      systems = [ "x86_64-linux" "aarch64-linux" ];
      mkHost = system: hostModule: nixpkgs.lib.nixosSystem {
        modules = [ disko.nixosModules.disko hostModule { nixpkgs.hostPlatform = system; } ];
      };
    in
    {
      nixosConfigurations = {
        # x86_64 only, for nix eval and repl; providers boot packages.<system>.vm-image.
        vm = mkHost "x86_64-linux" ./hosts/vm;
        # Installed by nixos-anywhere onto a stock Linux server (host/providers/hetzner.sh).
        hetzner = mkHost "x86_64-linux" ./hosts/hetzner;
      };

      packages = nixpkgs.lib.genAttrs systems (system: {
        vm-image = (mkHost system ./hosts/vm).config.system.build.image;
        run-vm = nixpkgs.legacyPackages.${system}.callPackage ./run-vm.nix { };
      });
    };
}
