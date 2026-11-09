{
  description = "One-time-use fundament/Gardener box on Hetzner Cloud";

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
      # host = the disko module + its host module (which pulls in the shared
      # modules/baseline.nix + modules/ephemeral-scratch.nix).
      mkHost = hostModule: nixpkgs.lib.nixosSystem {
        system = "x86_64-linux";
        modules = [ disko.nixosModules.disko hostModule ];
      };
    in
    {
      nixosConfigurations = {
        # One-time-use cloud box. Installed by `just up` (hetzner.sh), which
        # runs nixos-anywhere inside a throwaway nix container — so NO local nix is
        # needed even though this is a normal flake target.
        hetzner = mkHost ./hosts/hetzner;
      };
    };
}
