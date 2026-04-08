{
  description = "Candela — Open-source LLM observability platform";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        isLinux = pkgs.stdenv.isLinux;

        # Short rev from flake lock for deterministic image tagging.
        nixpkgsRev = builtins.substring 0 8 nixpkgs.rev;

        # Shared base utilities for CI containers.
        baseContents = with pkgs; [
          coreutils bash gnugrep gawk findutils gnused
          git cacert
        ];
      in
      {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            # Go
            go_1_26
            gopls
            golangci-lint
            gotools

            # Protobuf / Buf
            buf
            protobuf
            protoc-gen-go
            protoc-gen-go-grpc

            # Node.js (for web UI)
            nodejs_22
            pnpm

            # Python (for eval engine)
            python312
            python312Packages.pip
            python312Packages.virtualenv
            uv

            # Infrastructure tools
            docker-compose
            cloudflared
            grpcurl
            jq
            yq-go

            # Git
            git
            pre-commit
          ];

          shellHook = ''
            # Install pre-commit hooks (idempotent, silent).
            pre-commit install --install-hooks > /dev/null 2>&1 || true

            echo "🕯️  Candela dev shell ready"
            echo "   Go:     $(go version | cut -d' ' -f3)"
            echo "   Buf:    $(buf --version 2>&1)"
            echo "   Node:   $(node --version)"
            echo "   Python: $(python3 --version | cut -d' ' -f2)"
          '';
        };

        # ── CI container images (Linux only) ───────────────────────────
        # Built in CI via `nix build .#ci-go` / `nix build .#ci-ui`
        # and pushed to GHCR. Tagged with the nixpkgs rev for lockstep.
      } // pkgs.lib.optionalAttrs isLinux {
        packages.ci-go = pkgs.dockerTools.buildLayeredImage {
          name = "ghcr.io/candelahq/candela-ci-go";
          tag = nixpkgsRev;

          contents = baseContents ++ (with pkgs; [
            go_1_26
            golangci-lint
            buf
            protobuf
            protoc-gen-go
            protoc-gen-go-grpc
            nodejs_22  # needed for protoc-gen-es during buf generate
          ]);

          config = {
            Env = [
              "PATH=/bin:/usr/bin:/sbin:/usr/sbin"
              "GOPATH=/tmp/go"
              "GOFLAGS=-buildvcs=false"
              "SSL_CERT_FILE=${pkgs.cacert}/etc/ssl/certs/ca-bundle.crt"
            ];
            Labels = {
              "org.opencontainers.image.source" = "https://github.com/candelahq/candela";
            };
            WorkingDir = "/workspace";
          };
        };

        packages.ci-ui = pkgs.dockerTools.buildLayeredImage {
          name = "ghcr.io/candelahq/candela-ci-ui";
          tag = nixpkgsRev;

          contents = baseContents ++ (with pkgs; [
            nodejs_22
            buf
            protobuf
            chromium
          ]);

          config = {
            Env = [
              "PATH=/bin:/usr/bin:/sbin:/usr/sbin"
              "PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1"
              "PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH=${pkgs.chromium}/bin/chromium"
              "SSL_CERT_FILE=${pkgs.cacert}/etc/ssl/certs/ca-bundle.crt"
              "FONTCONFIG_FILE=${pkgs.fontconfig.out}/etc/fonts/fonts.conf"
            ];
            Labels = {
              "org.opencontainers.image.source" = "https://github.com/candelahq/candela";
            };
            WorkingDir = "/workspace";
          };
        };
      }
    );
}
