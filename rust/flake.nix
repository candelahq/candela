{
  description = "Candela Rust — LLM observability platform";

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
      in
      {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            # Rust
            rustToolchain
            cargo-deny
            cargo-watch

            # Protobuf / Buf
            buf
            protobuf

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

            echo "🕯️  Candela Rust dev shell ready"
            echo "   Rust:  $(rustc --version)"
            echo "   Cargo: $(cargo --version)"
            echo "   Buf:   $(buf --version 2>&1)"
          '';
        };
      }
    );
}
