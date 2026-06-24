# Candela Windows Native Release Spec

## Requirements

### Native CLI

Candela must build as `candela.exe` for `windows/amd64` and `windows/arm64`.

The Windows CLI must support the existing commands:

- `candela start`
- `candela stop`
- `candela restart`
- `candela status`
- `candela doctor`
- `candela run`
- `candela auth`
- `candela version`

### Background Process

`candela start` must launch `candela run` as a hidden background process on
Windows and write the same PID/log files used by the local CLI.

`candela stop` and `candela restart` must stop the tracked Windows process
without relying on Unix-only signals.

### Runtime Start Lifecycle

The embedded UI and `RuntimeService.StartRuntime` must explicitly start the
configured runtime backend exactly once per request, regardless of the
`runtime_manage.auto_start` setting.

Runtime health monitoring started by `RuntimeService.StartRuntime` must be tied
to the application lifecycle, not to the short-lived RPC request context, so the
health badge does not fall back to `STOPPED` after the start RPC completes.

Long-running runtime server processes launched by Candela must not be bound to
short-lived request contexts. Request contexts may bound readiness waiting, but
must not terminate a successfully launched LM Studio or vLLM server process.

### Doctor

`candela doctor` must detect Windows TCP listeners on the proxy and LM Studio
compat ports using native Windows tooling.

### Installer

The Windows MSI must be produced for `windows/amd64` and `windows/arm64`. It
must install `candela.exe` under `Program Files\Candela`, add that directory to
the system `PATH`, add Start Menu shortcuts for start/open UI/stop, support
silent install/uninstall, and remain unsigned for v1.

The public x64 MSI artifact must use the default Windows name
`Candela-<version>-windows.msi`; the Windows on Arm MSI artifact must use
`Candela-<version>-windows-arm64.msi`.

### Runtime Discovery

Windows runtime discovery must provide install hints for Ollama, LM Studio, and
vLLM. vLLM support requires a native Windows `vllm` executable on `PATH`; Candela
must not fallback to WSL.
