{
  description = "Hawk - AI coding agent powered by eyrie";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    # GrayCodeAI sibling repos — the public Go proxy has stale v0.1.0 tags
    # (post-history-rewrite), so resolve them locally like the Dockerfile.
    eyrie   = { url = "github:GrayCodeAI/eyrie";   flake = false; };
    inspect = { url = "github:GrayCodeAI/inspect"; flake = false; };
    sight   = { url = "github:GrayCodeAI/sight";   flake = false; };
    tok     = { url = "github:GrayCodeAI/tok";     flake = false; };
    trace   = { url = "github:GrayCodeAI/trace";   flake = false; };
    yaad    = { url = "github:GrayCodeAI/yaad";    flake = false; };
  };

  outputs = { self, nixpkgs, flake-utils, eyrie, inspect, sight, tok, trace, yaad }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        inherit (pkgs) lib;

        siblings = {
          "github.com/GrayCodeAI/eyrie"   = eyrie;
          "github.com/GrayCodeAI/inspect" = inspect;
          "github.com/GrayCodeAI/sight"   = sight;
          "github.com/GrayCodeAI/tok"     = tok;
          "github.com/GrayCodeAI/trace"   = trace;
          "github.com/GrayCodeAI/yaad"    = yaad;
        };

        # GrayCode modules currently follow github.com/<org>/<name>, so the
        # last path segment matches the sibling checkout directory name.
        dirOf = mod: lib.last (lib.splitString "/" mod);
        goVer = lib.removePrefix "go" "${pkgs.go_1_26.version}";

        # Copy sibling sources into external/, patch the go.mod go directive
        # to match the nixpkgs Go version, and add replace directives so the
        # build resolves siblings locally instead of hitting the stale proxy.
        setupReplace = ''
          sed -i 's/^go [0-9.]\+/go ${goVer}/' go.mod
          rm -f go.work go.work.sum
          mkdir -p external
          ${lib.concatStringsSep "\n" (lib.mapAttrsToList (mod: src:
            "cp -r ${src} external/${dirOf mod}"
          ) siblings)}
          ${lib.concatStringsSep "\n" (lib.mapAttrsToList (mod: _:
            "tmp_go_mod=$(mktemp) && awk '$0 != \"replace ${mod} => ./external/${dirOf mod}\"' go.mod > \"$tmp_go_mod\" && mv \"$tmp_go_mod\" go.mod"
          ) siblings)}
          ${lib.concatStringsSep "\n" (lib.mapAttrsToList (mod: _:
            "echo \"replace ${mod} => ./external/${dirOf mod}\" >> go.mod"
          ) siblings)}
        '';

        hawk = pkgs.buildGoModule rec {
          pname = "hawk";
          version = "0.1.0";

          src = ./.;

          # The public Go proxy has stale v0.1.0 tags for GrayCodeAI sibling
          # modules (post-history-rewrite). We resolve siblings locally via
          # replace directives in go.mod (added in preBuild below). External
          # deps (charmbracelet, cobra, …) are fetched from the proxy. The
          # vendor FOD is skipped (null) because the proxy version mismatch
          # prevents `go mod vendor` from passing checksum verification.
          # TODO(supply-chain): vendorHash = null + GONOSUMCHECK disables
          # dependency integrity/reproducibility for this build. Restore a
          # fixed-output vendorHash ("sha256-…", computed via `nix build`
          # after setupReplace) so external deps are verified, and drop
          # GONOSUMCHECK for non-GrayCodeAI modules.
          vendorHash = null;

          env = {
            GOPRIVATE = "github.com/GrayCodeAI/*";
            GONOSUMDB  = "github.com/GrayCodeAI/*";
            GONOSUMCHECK = "1";
            GOFLAGS = "-mod=mod";
          };

          # Add replace directives to go.mod so the build and vendor FOD
          # resolve sibling modules locally instead of from the stale proxy.
          preBuild = setupReplace;
          overrideModAttrs = old: {
            preBuild = setupReplace;
          };

          ldflags = [
            "-s"
            "-w"
            "-X main.Version=${version}"
          ];

          nativeBuildInputs = [ pkgs.git ];

          meta = with lib; {
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
          inherit hawk;
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go_1_26
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
