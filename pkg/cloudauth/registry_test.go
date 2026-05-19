package cloudauth

import (
	"testing"
)

func TestRegistry_Get_GCP(t *testing.T) {
	p, err := Get("gcp")
	if err != nil {
		t.Fatalf("Get(gcp): %v", err)
	}
	if p.Name() != "gcp" {
		t.Errorf("Name() = %q, want %q", p.Name(), "gcp")
	}
}

func TestRegistry_Get_Unknown(t *testing.T) {
	_, err := Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestRegistry_Names(t *testing.T) {
	names := Names()
	if len(names) == 0 {
		t.Fatal("Names() returned empty slice")
	}
	found := false
	for _, n := range names {
		if n == "gcp" {
			found = true
		}
	}
	if !found {
		t.Errorf("Names() = %v, expected to contain 'gcp'", names)
	}
}

func TestRegistry_All(t *testing.T) {
	providers := All()
	if len(providers) == 0 {
		t.Fatal("All() returned empty slice")
	}
}
