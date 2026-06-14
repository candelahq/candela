package costcalc

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
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

	snapshotPath := filepath.Join("testdata", "pricing_snapshot.json")

	if *updateSnapshot {
		if err := os.MkdirAll(filepath.Dir(snapshotPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(snapshotPath, append(current, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Log("Updated pricing snapshot")
		return
	}

	// If snapshot doesn't exist, create it and pass (first run).
	if _, err := os.Stat(snapshotPath); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(snapshotPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(snapshotPath, append(current, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Log("Created pricing snapshot — commit testdata/pricing_snapshot.json")
		return
	}

	// Compare with existing snapshot.
	expected, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}

	if string(append(current, '\n')) != string(expected) {
		// Write actual output for easy diffing.
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

func TestDefaultsModelCount(t *testing.T) {
	c := New()
	defaults := c.Defaults()

	// Sanity check: we should have a reasonable number of models.
	if len(defaults) < 20 {
		t.Errorf("only %d default models, expected at least 20", len(defaults))
	}

	// Check for duplicate entries.
	seen := make(map[string]bool)
	for _, d := range defaults {
		key := d.Provider + "/" + d.Model
		if seen[key] {
			t.Errorf("duplicate default pricing: %s", key)
		}
		seen[key] = true
	}
}
