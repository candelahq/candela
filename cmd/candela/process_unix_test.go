//go:build !windows

package main

import (
	"os"
	"testing"
)

func TestProcessRunning_Self(t *testing.T) {
	// The current process is always running.
	if !processRunning(os.Getpid()) {
		t.Fatal("processRunning(self) = false, want true")
	}
}

func TestProcessRunning_InvalidPID(t *testing.T) {
	if processRunning(0) {
		t.Fatal("processRunning(0) = true, want false")
	}
	if processRunning(-1) {
		t.Fatal("processRunning(-1) = true, want false")
	}
}

func TestProcessRunning_DeadPID(t *testing.T) {
	// PID 2147483647 (max int32) is extremely unlikely to be running.
	if processRunning(2147483647) {
		t.Fatal("processRunning(2147483647) = true, want false")
	}
}

func TestProcessRunning_PermissionDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("test requires non-root user (signal(0) to PID 1 returns EPERM only for non-root)")
	}
	// PID 1 (launchd on macOS, init/systemd on Linux) is always running and
	// owned by root. When we send signal(0) as a non-root user, the kernel
	// returns EPERM — the process exists but we lack permission.
	// processRunning must return true in this case.
	if !processRunning(1) {
		t.Fatal("processRunning(1) = false, want true (EPERM should be treated as running)")
	}
}
