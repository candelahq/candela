# Windows Native Release Design

## CLI Process Management

`cmd/candela` keeps the same public commands. OS-specific behavior is isolated
behind helper functions:

- Unix uses `Setsid`, `SIGTERM`, `SIGKILL`, `lsof`, and `launchctl`.
- Windows uses `CREATE_NEW_PROCESS_GROUP`, hidden background windows,
  `os.Process.Kill`, `OpenProcess` / `GetExitCodeProcess`, and PowerShell
  `Get-NetTCPConnection`.

This keeps the CLI command surface stable while making Windows compilation and
runtime checks native.

## Desktop UX

The MSI does not install a separate desktop shell. It installs `candela.exe` and
shortcuts:

- `Candela Start` runs `candela.exe start`.
- `Candela UI` opens `http://127.0.0.1:8181/_local/` in the user's default
  browser.
- `Candela Stop` runs `candela.exe stop`.

The optional desktop shortcut runs `candela.exe start`.

## Packaging

GoReleaser produces Windows zip artifacts as default `Windows` for Intel/AMD
x64 and `Windows_arm64` for Windows on Arm. A Windows
GitHub Actions matrix builds unsigned MSI installers with WiX for x64 and
Windows on Arm. The x64 MSI is smoke-tested with silent install/uninstall on the
hosted Windows runner; the Arm64 MSI is build-verified and uploaded with its
checksum.

## vLLM

Candela discovers `vllm` on Windows only when a native `vllm` executable is on
`PATH`. It does not rewrite commands to WSL and does not substitute a different
backend.
