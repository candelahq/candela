package runtime

import (
	"os/exec"
	"runtime"
)

// BackendInfo describes a runtime backend and its install status on this system.
type BackendInfo struct {
	// Name of the backend (e.g. "ollama").
	Name string
	// Installed is true if the binary was found on PATH.
	Installed bool
	// BinaryPath is the absolute path to the binary (if installed).
	BinaryPath string
	// InstallHint is a human-readable install instruction.
	InstallHint string
}

// backendMeta holds static metadata used for discovery.
type backendMeta struct {
	binary      string
	installHint map[string]string // GOOS → hint
}

// knownBackends maps backend names to their discovery metadata.
var knownBackends = map[string]backendMeta{
	"ollama": {
		binary: "ollama",
		installHint: map[string]string{
			"darwin": "brew install ollama",
			"linux":  "curl -fsSL https://ollama.com/install.sh | sh",
		},
	},
	"vllm": {
		binary: "vllm",
		installHint: map[string]string{
			"darwin": "pip install vllm",
			"linux":  "pip install vllm",
		},
	},
	"lmstudio": {
		binary: "lms",
		installHint: map[string]string{
			"darwin": "Download from https://lmstudio.ai",
			"linux":  "Download from https://lmstudio.ai",
		},
	},
}

// Discover scans the system PATH for known runtime binaries and returns
// their install status. Results include all registered backends regardless
// of whether they are installed.
func Discover() []BackendInfo {
	names := Names()
	infos := make([]BackendInfo, 0, len(names))

	for _, name := range names {
		info := BackendInfo{Name: name}

		meta, ok := knownBackends[name]
		if !ok {
			// Unknown backend — include but mark as not installed.
			infos = append(infos, info)
			continue
		}

		// Check if the binary exists on PATH.
		if path, err := exec.LookPath(meta.binary); err == nil {
			info.Installed = true
			info.BinaryPath = path
		}

		// Set platform-specific install hint.
		if hint, ok := meta.installHint[runtime.GOOS]; ok {
			info.InstallHint = hint
		} else {
			info.InstallHint = "See " + name + " documentation for install instructions"
		}

		infos = append(infos, info)
	}

	return infos
}
