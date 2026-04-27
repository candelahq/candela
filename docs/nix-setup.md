# 🧊 Development Environment Setup

Candela uses **Nix Flakes** for a fully reproducible dev environment. This means every contributor gets the exact same versions of Go, Node.js, Buf, Python, and all other tools — regardless of their OS or what's already installed.

> **TL;DR**: Install Nix → `cd candela` → `nix develop` → everything works.

---

## What Is Nix?

**Nix** is a purely functional package manager. Unlike Homebrew or apt, Nix:

- **Never conflicts** — each package is isolated in its own path (`/nix/store/...`)
- **Is reproducible** — the same inputs always produce the same outputs
- **Doesn't pollute your system** — uninstall Nix and everything is gone

You don't need to understand Nix deeply. Think of it as "Docker for your terminal" — it gives you a shell with all the right tools, without containers.

## What Are Nix Flakes?

**Flakes** are Nix's modern project definition format. Candela's `flake.nix` at the repo root defines:

- **`devShells.default`** — the dev shell with all tools (Go, Node, Buf, Python, etc.)
- **Lock file** (`flake.lock`) — pins exact versions of all dependencies

When you run `nix develop`, Nix reads `flake.nix`, downloads the exact pinned versions, and drops you into a shell where everything is available.

---

## Installing Nix

### Recommended: Determinate Nix Installer

The [Determinate Systems installer](https://docs.determinate.systems/determinate-nix/) is the easiest way to install Nix with Flakes enabled by default.

```bash
curl --proto '=https' --tlsv1.2 -sSf -L \
  https://install.determinate.systems/nix | sh -s -- install
```

This:
- Installs Nix with **Flakes and the unified CLI enabled by default**
- Works on macOS (Intel + Apple Silicon) and Linux
- Sets up the Nix daemon as a system service
- Is uninstallable: `/nix/nix-installer uninstall`

### Alternative: Official Nix Installer

If you prefer the official installer, you'll need to manually enable Flakes:

```bash
# Install Nix
sh <(curl -L https://nixos.org/nix/install)

# Enable flakes (add to ~/.config/nix/nix.conf)
echo "experimental-features = nix-command flakes" >> ~/.config/nix/nix.conf
```

### Verify Installation

```bash
nix --version
# nix (Nix) 2.x.x

# Enter the Candela dev shell
cd /path/to/candela
nix develop

# You should see:
# 🕯️  Candela dev shell ready
#    Go:     go1.26.1
#    Buf:    1.67.0
#    Node:   v22.22.2
#    Python: 3.12.13
```

---

## direnv — Automatic Shell Activation

Manually typing `nix develop` every time you `cd` into the repo is tedious. **direnv** automates this — it activates the Nix dev shell automatically when you enter the directory.

### What Is direnv?

[direnv](https://direnv.net/) is a shell extension that loads environment variables based on the current directory. When combined with Nix, it:

- **Auto-activates** the Nix dev shell when you `cd` into a project
- **Auto-deactivates** when you leave
- **Caches** the shell so re-entry is instant (no 10-second Nix eval)
- **Works with** bash, zsh, fish, and most editors (VS Code, Zed, etc.)

### Setup

#### 1. Install direnv

```bash
# macOS
brew install direnv

# Or via Nix (globally)
nix profile install nixpkgs#direnv
```

#### 2. Hook into your shell

Add to your shell config:

```bash
# ~/.zshrc (macOS default)
eval "$(direnv hook zsh)"

# ~/.bashrc
eval "$(direnv hook bash)"

# ~/.config/fish/config.fish
direnv hook fish | source
```

Restart your shell after adding the hook.

#### 3. Install nix-direnv (recommended)

**nix-direnv** dramatically improves caching. Without it, direnv re-evaluates the Nix expression on every shell entry.

```bash
# Add to ~/.config/direnv/direnvrc (create if needed)
mkdir -p ~/.config/direnv
cat >> ~/.config/direnv/direnvrc << 'EOF'
# Use nix-direnv for cached Nix shells
if type nix_direnv_version &>/dev/null; then
  nix_direnv_version 3.0.6
else
  source_url "https://raw.githubusercontent.com/nix-community/nix-direnv/3.0.6/direnvrc" \
    "sha256-RYcUJaRMf8oF5LznDrlCXbkOQrO8qIHIKjpZXb97gHc="
fi
EOF
```

#### 4. Allow the Candela `.envrc`

Candela ships a `.envrc` at the repo root. The first time you enter the directory, direnv will ask you to approve it:

```bash
cd /path/to/candela
# direnv: error /path/to/candela/.envrc is blocked. Run `direnv allow` to approve its content

direnv allow
# direnv: loading .envrc
# direnv: using flake
# 🕯️  Candela dev shell ready
```

After this, every time you `cd` into the repo (or open a terminal in your editor), the dev shell is automatically active.

### Editor Integration

Most editors support direnv natively or via plugins:

| Editor | Setup |
|--------|-------|
| **VS Code** | Install the [direnv extension](https://marketplace.visualstudio.com/items?itemName=mkhl.direnv) |
| **Zed** | Built-in direnv support (auto-detects `.envrc`) |
| **Neovim** | [direnv.vim](https://github.com/direnv/direnv.vim) plugin |
| **IntelliJ/GoLand** | [Direnv Integration](https://plugins.jetbrains.com/plugin/15285-direnv-integration) plugin |

---

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| `nix develop` hangs | First run downloading dependencies | Wait — initial download is ~1GB, subsequent runs are cached |
| `error: experimental Nix feature 'flakes' is disabled` | Flakes not enabled | Add `experimental-features = nix-command flakes` to `~/.config/nix/nix.conf` |
| `direnv: error .envrc is blocked` | First-time security prompt | Run `direnv allow` |
| `command not found: go` after `cd`-ing in | direnv not hooked into shell | Add `eval "$(direnv hook zsh)"` to `~/.zshrc` |
| Nix shell is slow to activate | nix-direnv not installed | Follow step 3 above for cached shells |
| `error: getting status of '/nix/store/...'` | Nix store corruption | Run `nix store verify --all --repair` |

---

## Without Nix (Not Recommended)

If you can't install Nix, you can manually install the required tools:

| Tool | Version | Install |
|------|---------|---------|
| Go | 1.26+ | [go.dev/dl](https://go.dev/dl/) |
| Node.js | 22+ | [nodejs.org](https://nodejs.org/) |
| pnpm | 10+ | `npm install -g pnpm` |
| Buf | 1.67+ | [buf.build/docs/installation](https://buf.build/docs/installation) |
| Python | 3.12+ | [python.org](https://www.python.org/) |
| uv | latest | `curl -LsSf https://astral.sh/uv/install.sh \| sh` |

> ⚠️ **Warning**: Without Nix, you're responsible for keeping tool versions in sync with the team. Pre-commit hooks may fail if your local versions differ from what CI uses.
