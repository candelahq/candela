package main

import (
	"path/filepath"
	"testing"
)

func TestStateDB_SettingsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := openStateDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Default: empty.
	if got := db.GetSetting("theme"); got != "" {
		t.Errorf("GetSetting(missing) = %q, want empty", got)
	}

	// Set and get.
	if err := db.SetSetting("theme", "dark"); err != nil {
		t.Fatal(err)
	}
	if got := db.GetSetting("theme"); got != "dark" {
		t.Errorf("GetSetting(theme) = %q, want dark", got)
	}

	// Overwrite.
	if err := db.SetSetting("theme", "light"); err != nil {
		t.Fatal(err)
	}
	if got := db.GetSetting("theme"); got != "light" {
		t.Errorf("GetSetting(theme) = %q, want light", got)
	}

	// Delete.
	if err := db.DeleteSetting("theme"); err != nil {
		t.Fatal(err)
	}
	if got := db.GetSetting("theme"); got != "" {
		t.Errorf("GetSetting(deleted) = %q, want empty", got)
	}
}

func TestStateDB_RuntimeState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := openStateDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Default state.
	rs := db.GetRuntimeState()
	if rs.Backend != "" {
		t.Errorf("default Backend = %q, want empty", rs.Backend)
	}

	// Set.
	if err := db.SetRuntimeState("ollama", "llama3.2:8b"); err != nil {
		t.Fatal(err)
	}
	rs = db.GetRuntimeState()
	if rs.Backend != "ollama" {
		t.Errorf("Backend = %q, want ollama", rs.Backend)
	}
	if rs.LastModel != "llama3.2:8b" {
		t.Errorf("LastModel = %q, want llama3.2:8b", rs.LastModel)
	}
	if rs.LastStarted.IsZero() {
		t.Error("LastStarted should not be zero after SetRuntimeState")
	}
}

func TestStateDB_PullHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := openStateDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Record pulls.
	if err := db.RecordPull("llama3.2:8b", "ollama", 4_700_000_000); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordPull("codellama:13b", "ollama", 7_300_000_000); err != nil {
		t.Fatal(err)
	}

	// Query recent.
	records, err := db.RecentPulls(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2", len(records))
	}
	// Most recent first.
	if records[0].Model != "codellama:13b" {
		t.Errorf("records[0].Model = %q, want codellama:13b", records[0].Model)
	}
	if records[0].SizeBytes != 7_300_000_000 {
		t.Errorf("records[0].SizeBytes = %d, want 7300000000", records[0].SizeBytes)
	}
}

func TestStateDB_Reset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := openStateDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Populate state.
	_ = db.SetSetting("theme", "dark")
	_ = db.SetRuntimeState("ollama", "llama3.2:8b")
	_ = db.RecordPull("llama3.2:8b", "ollama", 4_700_000_000)

	// Reset.
	if err := db.Reset(); err != nil {
		t.Fatal(err)
	}

	// Everything should be cleared.
	if got := db.GetSetting("theme"); got != "" {
		t.Errorf("after reset: theme = %q, want empty", got)
	}
	rs := db.GetRuntimeState()
	if rs.Backend != "" {
		t.Errorf("after reset: Backend = %q, want empty", rs.Backend)
	}
	records, _ := db.RecentPulls(10)
	if len(records) != 0 {
		t.Errorf("after reset: %d pull records, want 0", len(records))
	}
}

func TestStateDB_CreatesMissingDirs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deep", "nested", "state.db")
	db, err := openStateDB(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
}
