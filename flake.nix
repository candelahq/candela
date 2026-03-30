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
      in
      {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            # Go
            go_1_23
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
            nodePackages.pnpm

            # Python (for eval engine)
            python312
            python312Packages.pip
            python312Packages.virtualenv
            uv

            # Infrastructure tools
            docker-compose
            grpcurl
            jq
            yq-go

            # Git
            git
            pre-commit
          ];

          shellHook = ''
            echo "🕯️  Candela dev shell ready"
            echo "   Go:     $(go version | cut -d' ' -f3)"
            echo "   Buf:    $(buf --version 2>&1)"
            echo "   Node:   $(node --version)"
            echo "   Python: $(python3 --version | cut -d' ' -f2)"
          '';
        };
      }
    );
}
