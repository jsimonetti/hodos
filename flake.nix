{
  inputs.nixpkgs.url = "github:nixos/nixpkgs/nixpkgs-unstable";

  outputs = { self, nixpkgs }:
    let
      version = "0.0.1";
      supportedSystems = [ "x86_64-linux" "aarch64-linux" ];
      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;
      nixpkgsFor = forAllSystems (system: import nixpkgs { inherit system; });
    in
    {
      nixosModules = rec {
        hodos = import ./module.nix;
        default = hodos;
      };
      nixosModule = self.nixosModules.default;

      packages = forAllSystems (system:
        let
          pkgs = nixpkgsFor.${system};
        in
        {
          hodos = pkgs.callPackage ./default.nix {};
          default = self.packages.${system}.hodos;
        });

      devShells = forAllSystems (system:
        let
          pkgs = nixpkgsFor.${system};
        in
        {
          default = pkgs.mkShell {
            buildInputs = with pkgs; [ go gopls go-tools gotools act ];
          };
        });

      checks."x86_64-linux".integration = import ./test/integration.nix {
        inherit nixpkgs; pkgs = nixpkgs.legacyPackages."x86_64-linux";
        system = "x86_64-linux";
      };
    };
}
