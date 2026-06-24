//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

const (
	windowsStillActive             = 259
	processQueryLimitedInformation = 0x1000
)

func configureBackgroundCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
		HideWindow:    true,
	}
}

func processRunning(pid int) bool {
	if pid <= 0 {
		return false
	}

	handle, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		handle, err = syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION, false, uint32(pid))
		if err != nil {
			return false
		}
	}
	defer func() { _ = syscall.CloseHandle(handle) }()

	var exitCode uint32
	if err := syscall.GetExitCodeProcess(handle, &exitCode); err != nil {
		return false
	}
	return exitCode == windowsStillActive
}

func terminateProcess(process *os.Process) error {
	return process.Kill()
}

func forceKillProcess(process *os.Process) error {
	return process.Kill()
}

func isLaunchdManaged(_ int) bool {
	return false
}

func findProcessesOnPort(port int) []portProcessInfo {
	if port <= 0 {
		return nil
	}
	script := fmt.Sprintf(`$ErrorActionPreference = 'SilentlyContinue'
Get-NetTCPConnection -LocalPort %d -State Listen |
  Select-Object -ExpandProperty OwningProcess -Unique |
  ForEach-Object {
    $p = Get-Process -Id $_ -ErrorAction SilentlyContinue
    if ($null -ne $p) { "{0}{1}{2}" -f $p.Id, [char]9, $p.ProcessName }
  }`, port)

	out, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  Warning: failed to query TCP listeners with PowerShell: %v. Port conflict detection may be incomplete.\n", err)
		return nil
	}
	return parsePowerShellPortProcessOutput(out)
}

func parsePowerShellPortProcessOutput(out []byte) []portProcessInfo {
	var procs []portProcessInfo
	seen := make(map[int]bool)

	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil || seen[pid] {
			continue
		}
		seen[pid] = true
		procs = append(procs, portProcessInfo{pid: pid, command: fields[1]})
	}
	return procs
}
