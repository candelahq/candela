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
	ctrlBreakEvent                 = 1 // CTRL_BREAK_EVENT
)

var (
	kernel32                     = syscall.NewLazyDLL("kernel32.dll")
	procGenerateConsoleCtrlEvent = kernel32.NewProc("GenerateConsoleCtrlEvent")
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
		if err == syscall.ERROR_ACCESS_DENIED {
			return true
		}
		handle, err = syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION, false, uint32(pid))
		if err != nil {
			return err == syscall.ERROR_ACCESS_DENIED
		}
	}
	defer func() { _ = syscall.CloseHandle(handle) }()

	var exitCode uint32
	if err := syscall.GetExitCodeProcess(handle, &exitCode); err != nil {
		return false
	}
	return exitCode == windowsStillActive
}

// terminateProcess sends CTRL_BREAK_EVENT to the process group for graceful
// shutdown. This is the Windows equivalent of Unix SIGTERM. It requires the
// child to have been started with CREATE_NEW_PROCESS_GROUP (see
// configureBackgroundCommand).
func terminateProcess(process *os.Process) error {
	r1, _, err := procGenerateConsoleCtrlEvent.Call(
		uintptr(ctrlBreakEvent),
		uintptr(uint32(process.Pid)),
	)
	if r1 == 0 {
		return fmt.Errorf("GenerateConsoleCtrlEvent: %w", err)
	}
	return nil
}

// forceKillProcess unconditionally terminates the process (Win32
// TerminateProcess). Used as a fallback when graceful shutdown times out.
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
