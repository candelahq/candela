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

### Release Artifacts

The Windows release must publish ZIP artifacts for `windows/amd64` and
`windows/arm64`. The public x64 artifact must use the default Windows name
`candela_<version>_Windows.zip`; the Windows on Arm artifact must use
`candela_<version>_Windows_arm64.zip`.

The Windows ZIP artifacts must contain `candela.exe` and must support manual
startup with `candela.exe start`. This branch must not add a Windows installer,
desktop shell, or launcher management.

### Runtime Discovery

Windows runtime discovery must provide install hints for Ollama, LM Studio, and
vLLM. vLLM support requires a native Windows `vllm` executable on `PATH`; Candela
must not fallback to WSL.
