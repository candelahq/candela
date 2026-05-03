package prototest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// repoRoot returns the candela repo root by walking up from the test file.
func repoRoot(t *testing.T) string {
	t.Helper()
	// Walk up from the test binary's working dir to find go.mod.
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root (go.mod)")
		}
		dir = parent
	}
}

func TestIntegration_CI_NoProtoDir_Reference(t *testing.T) {
	root := repoRoot(t)
	ciPath := filepath.Join(root, ".github", "workflows", "ci.yml")

	data, err := os.ReadFile(ciPath)
	if err != nil {
		t.Fatalf("read ci.yml: %v", err)
	}

	content := string(data)
	if strings.Contains(content, "cd proto") {
		t.Error("ci.yml still contains 'cd proto' reference — proto/ has been deleted")
	}
	if strings.Contains(content, "proto/buf") {
		t.Error("ci.yml still contains 'proto/buf' reference — proto/ has been deleted")
	}
}

func TestIntegration_BufGenYaml_References_BSR(t *testing.T) {
	root := repoRoot(t)
	genPath := filepath.Join(root, "buf.gen.yaml")

	data, err := os.ReadFile(genPath)
	if err != nil {
		t.Fatalf("read buf.gen.yaml: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "buf.build/candelahq/protos") {
		t.Error("buf.gen.yaml does not reference BSR module buf.build/candelahq/protos")
	}
}

func TestIntegration_ProtoDir_DoesNotExist(t *testing.T) {
	root := repoRoot(t)
	protoDir := filepath.Join(root, "proto")

	if _, err := os.Stat(protoDir); err == nil {
		t.Error("proto/ directory still exists — should have been deleted in migration")
	}
}

func TestIntegration_PreCommitConfig_DoesNotExist(t *testing.T) {
	root := repoRoot(t)
	preCommitPath := filepath.Join(root, ".pre-commit-config.yaml")

	if _, err := os.Stat(preCommitPath); err == nil {
		t.Error(".pre-commit-config.yaml still exists — should have been replaced by lefthook.yml")
	}
}

func TestIntegration_Lefthook_Exists(t *testing.T) {
	root := repoRoot(t)
	lefthookPath := filepath.Join(root, "lefthook.yml")

	if _, err := os.Stat(lefthookPath); os.IsNotExist(err) {
		t.Error("lefthook.yml does not exist")
	}
}

func TestIntegration_Lefthook_HasSecurityHooks(t *testing.T) {
	root := repoRoot(t)
	lefthookPath := filepath.Join(root, "lefthook.yml")

	data, err := os.ReadFile(lefthookPath)
	if err != nil {
		t.Fatalf("read lefthook.yml: %v", err)
	}

	content := string(data)
	for _, hook := range []string{"detect-private-key", "check-merge-conflict"} {
		if !strings.Contains(content, hook) {
			t.Errorf("lefthook.yml missing security hook: %s", hook)
		}
	}
}

func TestIntegration_GeneratedStubs_Exist(t *testing.T) {
	root := repoRoot(t)

	// Check Go stubs exist
	goStubs := filepath.Join(root, "gen", "go", "candela", "v1", "user_service.pb.go")
	if _, err := os.Stat(goStubs); os.IsNotExist(err) {
		t.Error("generated Go stub user_service.pb.go does not exist")
	}

	// Check TS stubs exist
	tsDir := filepath.Join(root, "ui", "src", "gen")
	if _, err := os.Stat(tsDir); os.IsNotExist(err) {
		t.Skip("ui/src/gen/ not present — TS stubs may not be generated locally")
	}
}

func TestIntegration_GoImports_Compile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compilation test in short mode")
	}

	root := repoRoot(t)
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build ./... failed:\n%s", string(output))
	}
}
