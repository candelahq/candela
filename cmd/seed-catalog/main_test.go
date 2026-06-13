package main

import (
	"testing"

	"github.com/candelahq/candela/pkg/costcalc"
)

func TestDefaultsToEntries(t *testing.T) {
	calc := costcalc.New()
	defaults := calc.Defaults()

	if len(defaults) == 0 {
		t.Fatal("expected non-empty defaults")
	}

	// Verify sorted order (provider first, then model).
	for i := 1; i < len(defaults); i++ {
		prev := defaults[i-1]
		curr := defaults[i]
		if prev.Provider > curr.Provider ||
			(prev.Provider == curr.Provider && prev.Model > curr.Model) {
			t.Errorf("defaults not sorted at index %d: %s/%s > %s/%s",
				i, prev.Provider, prev.Model, curr.Provider, curr.Model)
		}
	}

}
