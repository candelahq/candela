//go:build windows

package runtime_test

import (
	"strings"
	"testing"

	"github.com/candelahq/candela/pkg/runtime"

	_ "github.com/candelahq/candela/pkg/runtime/lmstudio"
	_ "github.com/candelahq/candela/pkg/runtime/ollama"
	_ "github.com/candelahq/candela/pkg/runtime/vllm"
)

func TestDiscover_WindowsInstallHints(t *testing.T) {
	infos := runtime.Discover()
	hints := make(map[string]string, len(infos))
	for _, info := range infos {
		hints[info.Name] = info.InstallHint
	}

	tests := []struct {
		backend string
		want    string
	}{
		{"ollama", "winget install Ollama.Ollama"},
		{"lmstudio", "Download from https://lmstudio.ai"},
		{"vllm", "native Windows vLLM executable"},
	}

	for _, tt := range tests {
		t.Run(tt.backend, func(t *testing.T) {
			if !strings.Contains(hints[tt.backend], tt.want) {
				t.Fatalf("hint for %s = %q, want to contain %q", tt.backend, hints[tt.backend], tt.want)
			}
		})
	}
}
