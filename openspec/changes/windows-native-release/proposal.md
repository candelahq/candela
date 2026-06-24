# Windows Native Release

## Summary

Add a first-class Windows release path for the local `candela` binary. Windows
users should be able to install Candela as a native CLI/local desktop app,
start the proxy, and open the existing embedded management UI without any new
Electron, Tauri, or WebView shell.

## Motivation

The current release pipeline builds macOS and Linux archives, while the local
developer proxy already uses an embedded browser UI that can work on Windows.
Windows support needs native process management, port conflict discovery, and
release artifacts that are natural for Windows users.

## Scope

- Build `candela.exe` for `windows/amd64` and `windows/arm64`.
- Publish Windows zip artifacts through GoReleaser as default `Windows` for
  Intel/AMD x64 and `Windows_arm64` for Windows on Arm.
- Build and upload unsigned MSI installers for x64 and Windows on Arm.
- Add Start Menu shortcuts for starting Candela, stopping Candela, and opening
  the embedded UI.
- Keep the existing browser-based embedded UI.
- Support native Windows runtime discovery for Ollama, LM Studio, and vLLM when
  the relevant binaries are present on `PATH`.

## Non-Goals

- No Electron, Tauri, Wails, Flutter, or WebView shell in this change.
- No vLLM vendoring or WSL fallback.
- No Windows code signing in the first MSI release.
