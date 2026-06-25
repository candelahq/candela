//go:build windows

package main

import (
	"os"
	"os/exec"
	"syscall"
	"testing"
)

func TestParsePowerShellPortProcessOutput(t *testing.T) {
	out := []byte("1234\tcandela\r\n5678\tollama\n1234\tcandela\n")

	procs := parsePowerShellPortProcessOutput(out)

	if len(procs) != 2 {
		t.Fatalf("len(procs) = %d, want 2", len(procs))
	}
	if procs[0].pid != 1234 || procs[0].command != "candela" {
		t.Fatalf("procs[0] = %+v, want candela PID 1234", procs[0])
	}
	if procs[1].pid != 5678 || procs[1].command != "ollama" {
		t.Fatalf("procs[1] = %+v, want ollama PID 5678", procs[1])
	}
}

func TestConfigureBackgroundCommandWindows(t *testing.T) {
	cmd := exec.Command("candela.exe", "run")

	configureBackgroundCommand(cmd)

	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr is nil")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Fatal("HideWindow = false, want true")
	}
	if cmd.SysProcAttr.CreationFlags&syscall.CREATE_NEW_PROCESS_GROUP == 0 {
		t.Fatalf("CreationFlags = %#x, missing CREATE_NEW_PROCESS_GROUP", cmd.SysProcAttr.CreationFlags)
	}
}

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

func TestProcessRunning_AccessDenied(t *testing.T) {
	// PID 4 is the Windows "System" process — always running, always owned
	// by NT AUTHORITY\SYSTEM. OpenProcess returns ERROR_ACCESS_DENIED for
	// normal (non-elevated) users. processRunning must return true because
	// access denied proves the process exists.
	//
	// If running as SYSTEM/elevated admin, OpenProcess may succeed and
	// GetExitCodeProcess returns STILL_ACTIVE, so the test still passes.
	if !processRunning(4) {
		t.Fatal("processRunning(4) = false, want true (System process should always be detected as running)")
	}
}
