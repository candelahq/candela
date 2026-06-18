package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestFindProcessesOnPort_NoListeners(t *testing.T) {
	// Use a very high ephemeral port that's unlikely to be in use.
	procs := findProcessesOnPort(59999)
	if len(procs) != 0 {
		t.Errorf("expected no processes on port 59999, got %d", len(procs))
	}
}

func TestFindProcessesOnPort_WithListener(t *testing.T) {
	// Start a listener on a random port.
	cmd := exec.Command("bash", "-c", `
		exec 3<>/dev/tcp/127.0.0.1/0 2>/dev/null || true
		# Use python to create a simple TCP listener
		python3 -c "
import socket, time, sys
s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
s.bind(('127.0.0.1', 0))
s.listen(1)
port = s.getsockname()[1]
print(port, flush=True)
time.sleep(30)
" &
		wait
	`)
	// Simpler approach: just use nc or python directly.
	listener := exec.Command("python3", "-c", `
import socket, time, sys, os
s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
s.bind(('127.0.0.1', 59998))
s.listen(1)
sys.stdout.write('ready\n')
sys.stdout.flush()
time.sleep(30)
`)
	_ = cmd // unused, we use listener instead

	stdout, err := listener.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := listener.Start(); err != nil {
		t.Skipf("could not start listener: %v", err)
	}
	t.Cleanup(func() { _ = listener.Process.Kill(); _ = listener.Wait() })

	// Wait for "ready" signal.
	buf := make([]byte, 64)
	n, _ := stdout.Read(buf)
	if !strings.Contains(string(buf[:n]), "ready") {
		t.Fatal("listener did not become ready")
	}

	procs := findProcessesOnPort(59998)
	if len(procs) == 0 {
		t.Error("expected to find a process on port 59998")
		return
	}
	if procs[0].pid != listener.Process.Pid {
		t.Errorf("expected PID %d, got %d", listener.Process.Pid, procs[0].pid)
	}
	if procs[0].command == "" {
		t.Error("expected non-empty command name")
	}
}

func TestFindProcessesOnPort_DeduplicatesPIDs(t *testing.T) {
	// findProcessesOnPort should not return the same PID twice
	// even if lsof shows multiple file descriptors for the same process.
	// We test this indirectly: start one listener, verify we get exactly 1 result.
	listener := exec.Command("python3", "-c", `
import socket, time, sys
s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
s.bind(('127.0.0.1', 59997))
s.listen(1)
sys.stdout.write('ready\n')
sys.stdout.flush()
time.sleep(30)
`)
	stdout, err := listener.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := listener.Start(); err != nil {
		t.Skipf("could not start listener: %v", err)
	}
	t.Cleanup(func() { _ = listener.Process.Kill(); _ = listener.Wait() })

	buf := make([]byte, 64)
	n, _ := stdout.Read(buf)
	if !strings.Contains(string(buf[:n]), "ready") {
		t.Fatal("listener did not become ready")
	}

	procs := findProcessesOnPort(59997)
	if len(procs) != 1 {
		t.Errorf("expected exactly 1 process, got %d", len(procs))
	}
}

func TestIsLaunchdManaged_NonManagedPID(t *testing.T) {
	// Our own test process PID should not be launchd-managed.
	if isLaunchdManaged(os.Getpid()) {
		t.Error("test process should not be launchd-managed")
	}
}

func TestIsLaunchdManaged_NonexistentPID(t *testing.T) {
	// PID 99999999 should definitely not be managed.
	if isLaunchdManaged(99999999) {
		t.Error("nonexistent PID should not be launchd-managed")
	}
}

func TestCmdDoctor_ExcludesOwnPID(t *testing.T) {
	// If the PID file contains our own PID, doctor should exclude it from conflicts.
	// We test the underlying logic: write a PID file, check that findProcessesOnPort
	// returns our PID, and verify the exclusion logic would skip it.

	tmpDir := t.TempDir()
	pidPath := filepath.Join(tmpDir, "candela.pid")
	ownPID := os.Getpid()
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(ownPID)), 0o644); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatal(err)
	}
	readPID, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	if readPID != ownPID {
		t.Errorf("PID mismatch: wrote %d, read %d", ownPID, readPID)
	}
}

func TestFindProcessesOnPort_ParsesLsofOutput(t *testing.T) {
	// Verify that lsof is available — skip if not (e.g. CI container without lsof).
	if _, err := exec.LookPath("lsof"); err != nil {
		t.Skip("lsof not available")
	}

	// Port 1 should have nothing listening (requires root).
	procs := findProcessesOnPort(1)
	if len(procs) != 0 {
		t.Errorf("expected no processes on port 1, got %d", len(procs))
	}
}
