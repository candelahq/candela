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
            actionlint

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

        # CI dev container — baked from the same Nix inputs as the dev shell.
        # Build: nix build .#devContainer
        # Push:  docker load < result && docker push ghcr.io/candelahq/candela-dev:latest
        packages.devContainer = pkgs.dockerTools.buildLayeredImage {
          name = "candela-dev";
          tag  = "latest";

          # Include only what CI jobs need — keeps the image lean.
          contents = with pkgs; [
            # Core
            bashInteractive
            coreutils
            cacert         # TLS certs (needed for go get, gcloud, etc.)
            git
            curl
            jq

            # Go toolchain
            go_1_26
            golangci-lint
            gotools
            govulncheck
            actionlint

            # Protobuf / Buf (needed for buf generate in every job)
            buf
            protobuf
            protoc-gen-bq-schema

            # Node + pnpm (for UI jobs)
            nodejs_22
            pnpm

            # GCP tooling (Firestore emulator)
            google-cloud-sdk

            # Rust (for rust-check job)
            rustToolchain
            cargo-deny
          ];

          config = {
            Env = [
              # Point Go module cache to a writable path inside the container.
              "GOPATH=/root/go"
              "GOMODCACHE=/root/go/pkg/mod"
              "PATH=/root/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
              # Trust the Nix-provided CA bundle.
              "SSL_CERT_FILE=${pkgs.cacert}/etc/ssl/certs/ca-bundle.crt"
            ];
          };
        };
      }
    );
}
