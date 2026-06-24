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

## Packaging

GoReleaser produces Windows zip artifacts as default `Windows` for Intel/AMD
x64 and `Windows_arm64` for Windows on Arm. Users extract the ZIP, optionally
add the extracted directory to `PATH`, run `candela.exe start`, and open
`http://127.0.0.1:8181/_local/` in their browser.

This branch does not build an installer or manage desktop launcher integration.
Installed desktop UX belongs in a separate desktop-specific branch.

## vLLM

Candela discovers `vllm` on Windows only when a native `vllm` executable is on
`PATH`. It does not rewrite commands to WSL and does not substitute a different
backend.
