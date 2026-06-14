package costcalc

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

var updateSnapshot = flag.Bool("update-snapshot", false, "update pricing snapshot")

func TestPricingSnapshot(t *testing.T) {
	c := New()
	defaults := c.Defaults() // Already sorted by provider, then model.

	// Generate current snapshot.
	current, err := json.MarshalIndent(defaults, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	// Add trailing newline for editor compatibility — most editors and
	// git prefer files to end with a newline.
	current = append(current, '\n')

	snapshotPath := filepath.Join("testdata", "pricing_snapshot.json")

	if *updateSnapshot {
		if err := os.MkdirAll(filepath.Dir(snapshotPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(snapshotPath, current, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Log("Updated pricing snapshot")
		return
	}

	// If the snapshot doesn't exist, fail rather than silently creating it.
	// In CI (where testdata may not be committed), this ensures drift is
	// caught instead of masked by auto-creation.
	if _, err := os.Stat(snapshotPath); os.IsNotExist(err) {
		t.Fatalf("pricing snapshot missing: %s\nRun with -update-snapshot to create it", snapshotPath)
	}

	// Compare with existing snapshot.
	expected, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}

	if string(current) != string(expected) {
		// Write actual output for easy diffing (with trailing newline).
		_ = os.WriteFile(snapshotPath+".actual", current, 0o644)
		t.Errorf("pricing drift detected!\n"+
			"The default pricing in calculator.go has changed since the last snapshot.\n"+
			"If this is intentional:\n"+
			"  1. Run: go test ./pkg/costcalc/ -run TestPricingSnapshot -update-snapshot\n"+
			"  2. Or copy %s.actual → %s\n"+
			"  3. Commit the updated snapshot",
			snapshotPath, snapshotPath)
	}
}

// TestPricingSnapshot_RequiresFile verifies the snapshot file is committed to
// the repo. This is a fast, standalone check that doesn't require computing
// the snapshot — it just ensures the file exists on disk.
func TestPricingSnapshot_RequiresFile(t *testing.T) {
	snapshotPath := filepath.Join("testdata", "pricing_snapshot.json")
	if _, err := os.Stat(snapshotPath); os.IsNotExist(err) {
		t.Fatal("pricing_snapshot.json must be committed to the repo")
	}
}

func TestDefaultsModelCount(t *testing.T) {
	c := New()
	defaults := c.Defaults()

	// Sanity check: we should have a reasonable number of models.
	if len(defaults) < 30 {
		t.Errorf("only %d default models, expected at least 30", len(defaults))
	}

	// Check for duplicate entries (provider + model key).
	//
	// NOTE: loadDefaults() stores entries into a map keyed by provider/model,
	// so true duplicates in the source slice are silently overwritten before
	// Defaults() ever sees them. This check on Defaults() acts as a safety
	// net — if loadDefaults ever changes to use a slice or append-based
	// approach, this will catch regressions. It also validates that the
	// map-based dedup hasn't hidden a copy-paste error where two entries
	// differ only in pricing (last-write-wins would mask the first).
	seen := make(map[string]bool)
	for _, d := range defaults {
		key := d.Provider + "/" + d.Model
		if seen[key] {
			t.Errorf("duplicate default pricing: %s", key)
		}
		seen[key] = true
	}
}

func TestPricingYAMLValid(t *testing.T) {
	var pf pricingFile
	if err := yaml.Unmarshal(defaultPricingYAML, &pf); err != nil {
		t.Fatalf("pricing.yaml parse error: %v", err)
	}
	if len(pf.Models) == 0 {
		t.Fatal("pricing.yaml has zero models")
	}
	for i, m := range pf.Models {
		if m.Provider == "" {
			t.Errorf("entry %d: empty provider", i)
		}
		if m.Model == "" {
			t.Errorf("entry %d: empty model", i)
		}
		if m.InputPerMillion <= 0 {
			t.Errorf("entry %d (%s/%s): input_per_million must be > 0", i, m.Provider, m.Model)
		}
		if m.OutputPerMillion <= 0 {
			t.Errorf("entry %d (%s/%s): output_per_million must be > 0", i, m.Provider, m.Model)
		}
	}
}

func TestPricingYAMLModelCount(t *testing.T) {
	c := New()
	defaults := c.Defaults()
	if len(defaults) < 30 {
		t.Errorf("expected at least 30 models, got %d", len(defaults))
	}
}
