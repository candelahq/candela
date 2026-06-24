# Candela Windows Native Release Spec

## ADDED Requirements

### Requirement: Native Windows CLI

Candela SHALL build as `candela.exe` for `windows/amd64` and `windows/arm64`.

#### Scenario: Windows binary exposes existing commands

- **GIVEN** a Windows `candela.exe` build
- **WHEN** a user invokes the CLI
- **THEN** the binary SHALL support `start`, `stop`, `restart`, `status`, `doctor`, `run`, `auth`, and `version`.

### Requirement: Windows Background Process

`candela start` SHALL launch `candela run` as a hidden background process on
Windows and write the same PID/log files used by the local CLI.

#### Scenario: Start creates a tracked background process

- **GIVEN** a Windows user runs `candela start`
- **WHEN** Candela launches the local proxy
- **THEN** it SHALL start a hidden `candela run` process and persist the PID/log files used by `status`, `stop`, and `restart`.

#### Scenario: Stop avoids Unix-only signals

- **GIVEN** a tracked Windows process exists
- **WHEN** a user runs `candela stop` or `candela restart`
- **THEN** Candela SHALL stop the process without relying on Unix-only signals.

### Requirement: Runtime Start Lifecycle

The embedded UI and `RuntimeService.StartRuntime` SHALL explicitly start the
configured runtime backend exactly once per request, regardless of the
`runtime_manage.auto_start` setting.

Runtime health monitoring started by `RuntimeService.StartRuntime` SHALL be tied
to the application lifecycle, not to the short-lived RPC request context, so the
health badge does not fall back to `STOPPED` after the start RPC completes.

Long-running runtime server processes launched by Candela SHALL NOT be bound to
short-lived request contexts. Request contexts may bound readiness waiting, but
must not terminate a successfully launched LM Studio or vLLM server process.

Runtime manager lifecycle state, including the active health-loop cancel
function, SHALL be synchronized so concurrent start and stop requests do not
race.

#### Scenario: UI start keeps monitoring after RPC completion

- **GIVEN** the embedded UI calls `RuntimeService.StartRuntime`
- **WHEN** the start RPC returns successfully
- **THEN** runtime health monitoring SHALL continue under the application context.

#### Scenario: Concurrent lifecycle requests do not race cancellation state

- **GIVEN** multiple callers invoke runtime start and stop operations concurrently
- **WHEN** Candela replaces or cancels the active health-loop context
- **THEN** access to the stored cancel function SHALL be synchronized.

### Requirement: Windows Doctor Port Discovery

`candela doctor` SHALL detect Windows TCP listeners on the proxy and LM Studio
compat ports using native Windows tooling.

#### Scenario: Doctor reports Windows port listeners

- **GIVEN** a Windows process is listening on a Candela-relevant port
- **WHEN** a user runs `candela doctor`
- **THEN** the diagnostic output SHALL identify the listener using Windows-native tooling.

### Requirement: Windows Release Artifacts

The Windows release SHALL publish ZIP artifacts for `windows/amd64` and
`windows/arm64`. The public x64 artifact SHALL use the default Windows name
`candela_<version>_Windows.zip`; the Windows on Arm artifact SHALL use
`candela_<version>_Windows_arm64.zip`.

The Windows ZIP artifacts SHALL contain `candela.exe` and SHALL support manual
startup with `candela.exe start`. This branch SHALL NOT add a Windows installer,
desktop shell, or launcher management.

#### Scenario: Release includes zip-only Windows artifacts

- **GIVEN** a release is built
- **WHEN** GoReleaser creates Windows artifacts
- **THEN** it SHALL publish `candela_<version>_Windows.zip` and `candela_<version>_Windows_arm64.zip` containing `candela.exe`.

### Requirement: Windows Runtime Discovery

Windows runtime discovery SHALL provide install hints for Ollama, LM Studio, and
vLLM. vLLM support requires a native Windows `vllm` executable on `PATH`;
Candela SHALL NOT fallback to WSL.

#### Scenario: vLLM discovery requires native Windows executable

- **GIVEN** Candela is running on Windows
- **WHEN** `vllm` is not available as a native executable on `PATH`
- **THEN** Candela SHALL report an install hint and SHALL NOT attempt a WSL/Linux fallback.
