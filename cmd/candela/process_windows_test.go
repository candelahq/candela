//go:build windows

package main

import (
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
