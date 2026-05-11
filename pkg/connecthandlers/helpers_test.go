package connecthandlers

// Tests for new helper functions (stringPtr, derefString, derefInt) and the
// pointer-field semantics of UpdateUser introduced in the billing-hardening push.

import (
	"testing"
)

// ─── stringPtr ────────────────────────────────────────────────────────────────

func TestStringPtr_ReturnsNonNilPointer(t *testing.T) {
	s := "hello"
	p := stringPtr(s)
	if p == nil {
		t.Fatal("stringPtr returned nil")
	}
	if *p != "hello" {
		t.Errorf("*p = %q, want hello", *p)
	}
}

func TestStringPtr_EmptyStringIsNonNil(t *testing.T) {
	p := stringPtr("")
	if p == nil {
		t.Fatal("stringPtr(\"\") returned nil — should return &\"\"")
	}
}

func TestStringPtr_DistinctPointers(t *testing.T) {
	// Two calls should return independent pointers.
	p1 := stringPtr("x")
	p2 := stringPtr("x")
	if p1 == p2 {
		t.Error("stringPtr returned the same pointer for two calls — not independent")
	}
}

// ─── derefString ──────────────────────────────────────────────────────────────

func TestDerefString_Nil(t *testing.T) {
	if got := derefString(nil); got != "" {
		t.Errorf("derefString(nil) = %q, want empty string", got)
	}
}

func TestDerefString_NonNil(t *testing.T) {
	s := "world"
	if got := derefString(&s); got != "world" {
		t.Errorf("derefString(&s) = %q, want world", got)
	}
}

func TestDerefString_Empty(t *testing.T) {
	s := ""
	if got := derefString(&s); got != "" {
		t.Errorf("derefString(&\"\") = %q, want empty", got)
	}
}

// ─── derefInt ─────────────────────────────────────────────────────────────────

func TestDerefInt_Nil(t *testing.T) {
	if got := derefInt(nil); got != 0 {
		t.Errorf("derefInt(nil) = %d, want 0", got)
	}
}

func TestDerefInt_NonNil(t *testing.T) {
	i := 42
	if got := derefInt(&i); got != 42 {
		t.Errorf("derefInt(&42) = %d, want 42", got)
	}
}

func TestDerefInt_Zero(t *testing.T) {
	i := 0
	if got := derefInt(&i); got != 0 {
		t.Errorf("derefInt(&0) = %d, want 0", got)
	}
}
