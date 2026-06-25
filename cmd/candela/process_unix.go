//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func configureBackgroundCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

func processRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	// EPERM means the process exists but belongs to another user.
	if err == syscall.EPERM {
		return true
	}
	return false
}

func terminateProcess(process *os.Process) error {
	return process.Signal(syscall.SIGTERM)
}

func forceKillProcess(process *os.Process) error {
	return process.Signal(syscall.SIGKILL)
}

// isLaunchdManaged checks whether a PID is managed by a launchd service
// (e.g. homebrew.mxcl.candela). Killing a launchd-managed process will
// cause launchd to respawn it immediately.
func isLaunchdManaged(pid int) bool {
	out, err := exec.Command("launchctl", "list").Output()
	if err != nil {
		return false
	}
	pidStr := strconv.Itoa(pid)
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == pidStr {
			label := fields[2]
			if strings.Contains(label, "candela") || strings.Contains(label, "homebrew") {
				return true
			}
		}
	}
	return false
}

// findProcessesOnPort uses lsof to find processes listening on a given TCP port.
func findProcessesOnPort(port int) []portProcessInfo {
	out, err := exec.Command("lsof", "-i", fmt.Sprintf("tcp:%d", port), "-sTCP:LISTEN", "-n", "-P").Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil // no processes found
		}
		fmt.Fprintf(os.Stderr, "⚠️  Warning: failed to run 'lsof': %v. Port conflict detection may be incomplete.\n", err)
		return nil
	}

	var procs []portProcessInfo
	seen := make(map[int]bool)
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] == "COMMAND" {
			continue
		}
		pid, err := strconv.Atoi(fields[1])
		if err != nil || seen[pid] {
			continue
		}
		seen[pid] = true
		procs = append(procs, portProcessInfo{pid: pid, command: fields[0]})
	}
	return procs
}
