{
  description = "Hawk - AI coding agent powered by eyrie";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        
        hawk = pkgs.buildGoModule rec {
          pname = "hawk";
          version = "0.1.0";
          
          src = ./.;
          
          vendorHash = null;
          
          ldflags = [
            "-s"
            "-w"
            "-X main.Version=${version}"
          ];
          
          nativeBuildInputs = [ pkgs.git ];
          
          meta = with pkgs.lib; {
            description = "AI coding agent that reads, writes, and runs code in your terminal";
            homepage = "https://github.com/GrayCodeAI/hawk";
            license = licenses.mit;
            maintainers = [ ];
          };
        };
      in
      {
        packages = {
          default = hawk;
          hawk = hawk;
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go_1_23
            gopls
            gotools
            go-tools
            golangci-lint
            git
          ];

          shellHook = ''
            echo "Hawk development shell"
            echo "Go version: $(go version)"
          '';
        };

        apps.default = {
          type = "app";
          program = "${hawk}/bin/hawk";
        };
      });
}
