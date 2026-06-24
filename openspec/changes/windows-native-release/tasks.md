# Windows Native Release Tasks

- [x] Split `cmd/candela` process and port helpers by OS.
- [x] Add Windows tests for process flags and PowerShell port parsing.
- [x] Make `TestStopProcess` cross-platform.
- [x] Add Windows runtime discovery hints.
- [x] Add GoReleaser `Windows` x64 and `Windows_arm64` artifacts.
- [x] Add Windows CI smoke tests.
- [x] Keep Windows release flow zip-only and defer installer/launcher work.
- [x] Fix explicit runtime start lifecycle so UI Start does not fall back to stopped after the RPC request completes.
- [x] Guard runtime manager health-loop cancellation state against concurrent start and stop races.
- [x] Update user-facing documentation.
- [x] Run final local verification.
