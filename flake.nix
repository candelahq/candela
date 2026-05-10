{
  description = "Candela — Open-source LLM observability platform";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    rust-overlay = {
      url = "github:oxalica/rust-overlay";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs = { self, nixpkgs, flake-utils, rust-overlay }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        overlays = [ (import rust-overlay) ];
        pkgs = import nixpkgs { inherit system overlays; };

        rustToolchain = pkgs.rust-bin.stable.latest.default.override {
          extensions = [ "rust-src" "rust-analyzer" ];
        };

        # BigQuery schema generator for protobuf (not in nixpkgs).
        protoc-gen-bq-schema = pkgs.buildGoModule rec {
          pname = "protoc-gen-bq-schema";
          version = "3.1.0";
          src = pkgs.fetchFromGitHub {
            owner = "GoogleCloudPlatform";
            repo = "protoc-gen-bq-schema";
            rev = "v${version}";
            sha256 = "sha256-fsnCGv9C5S+/VcBr88IXZTvgcMacQ2x4fnV3NyHrwSk=";
          };
          vendorHash = "sha256-nGAX1r6JgjZ0w9McpICd8nP+oWqu9PY6hSqTztm3s70=";
        };
      in
      {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            # Go
            go_1_26
            gopls
            golangci-lint
            gotools
            govulncheck

            # Protobuf / Buf
            buf
            protobuf
            protoc-gen-bq-schema

            # Node.js (for web UI)
            nodejs_22
            pnpm

            # Python (for eval engine)
            python312
            python312Packages.pip
            python312Packages.virtualenv
            uv

            # Infrastructure tools
            opentofu    # Open-source Terraform fork (BSL-free, same CLI as `tofu`)
            docker-compose
            cloudflared
            grpcurl
            google-cloud-sdk  # Includes Firestore emulator (gcloud emulators firestore start)
            jq
            yq-go

            # Rust
            rustToolchain
            cargo-deny
            cargo-watch

            # Git
            git
            gh
            lefthook
          ];

          shellHook = ''
            # Install lefthook git hooks (only if not already present)
            if [ -d .git ] && ! grep -q 'LEFTHOOK' .git/hooks/pre-commit 2>/dev/null; then
              lefthook install 2>/dev/null
            fi

            echo "🕯️  Candela dev shell ready"
            echo "   Go:     $(go version | cut -d' ' -f3)"
            echo "   Rust:   $(rustc --version | cut -d' ' -f2)"
            echo "   Buf:    $(buf --version 2>&1)"
            echo "   Node:   $(node --version)"
            echo "   Python: $(python3 --version | cut -d' ' -f2)"
          '';
        };
      }
    );
}
