package runtime_test

import (
	"testing"

	"github.com/candelahq/candela/pkg/runtime"

	// Register backends so Names() returns entries.
	_ "github.com/candelahq/candela/pkg/runtime/lmstudio"
	_ "github.com/candelahq/candela/pkg/runtime/ollama"
	_ "github.com/candelahq/candela/pkg/runtime/vllm"
)

func TestDiscover_ReturnsAllBackends(t *testing.T) {
	infos := runtime.Discover()

	// We should get at least 3 backends (ollama, vllm, lmstudio).
	if len(infos) < 3 {
		t.Fatalf("Discover() returned %d backends, want >= 3", len(infos))
	}

	// All registered backends must appear in the output.
	names := make(map[string]bool)
	for _, info := range infos {
		names[info.Name] = true
	}
	for _, want := range []string{"ollama", "vllm", "lmstudio"} {
		if !names[want] {
			t.Errorf("Discover() missing backend %q", want)
		}
	}
}

func TestDiscover_HasInstallHints(t *testing.T) {
	infos := runtime.Discover()

	for _, info := range infos {
		if info.InstallHint == "" {
			t.Errorf("backend %q has no install hint", info.Name)
		}
	}
}

func TestDiscover_InstalledBackendHasPath(t *testing.T) {
	infos := runtime.Discover()

	for _, info := range infos {
		if info.Installed && info.BinaryPath == "" {
			t.Errorf("backend %q is installed but has no binary path", info.Name)
		}
	}
}
