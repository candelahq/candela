package proxy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/storage"
)

// mockModelLimitStore implements modelLimitStore for testing.
type mockModelLimitStore struct {
	limits map[string][]*storage.ModelLimitRecord
	err    error
	calls  int
}

func (m *mockModelLimitStore) GetModelLimits(_ context.Context, userID string) ([]*storage.ModelLimitRecord, error) {
	m.calls++
	if m.err != nil {
		return nil, m.err
	}
	return m.limits[userID], nil
}

func TestModelLimitCache_HitWithinTTL(t *testing.T) {
	store := &mockModelLimitStore{
		limits: map[string][]*storage.ModelLimitRecord{
			"alice": {
				{UserID: "alice", ModelPrefix: "claude-opus-4", MaxDailyUSD: 10},
			},
		},
	}
	cache := newModelLimitCache(store, 60*time.Second)

	// First call — cache miss, fetches from store.
	limits := cache.getUserLimits(context.Background(), "alice")
	if len(limits) != 1 || limits[0].MaxDailyUSD != 10 {
		t.Fatalf("first call: got %+v, want [{claude-opus-4, 10}]", limits)
	}
	if store.calls != 1 {
		t.Fatalf("first call: store.calls = %d, want 1", store.calls)
	}

	// Second call — cache hit, no additional store call.
	limits = cache.getUserLimits(context.Background(), "alice")
	if len(limits) != 1 {
		t.Fatalf("second call: got %d limits, want 1", len(limits))
	}
	if store.calls != 1 {
		t.Fatalf("second call: store.calls = %d, want 1 (cached)", store.calls)
	}
}

func TestModelLimitCache_ExpiredTTL(t *testing.T) {
	store := &mockModelLimitStore{
		limits: map[string][]*storage.ModelLimitRecord{
			"alice": {
				{UserID: "alice", ModelPrefix: "gpt-4o", MaxDailyUSD: 5},
			},
		},
	}
	// Use a very short TTL so it expires immediately.
	cache := newModelLimitCache(store, 1*time.Nanosecond)

	_ = cache.getUserLimits(context.Background(), "alice")
	if store.calls != 1 {
		t.Fatalf("first call: store.calls = %d, want 1", store.calls)
	}

	// Wait for TTL to expire.
	time.Sleep(10 * time.Millisecond)

	_ = cache.getUserLimits(context.Background(), "alice")
	if store.calls != 2 {
		t.Fatalf("after TTL: store.calls = %d, want 2", store.calls)
	}
}

func TestModelLimitCache_StoreError_FailOpen(t *testing.T) {
	store := &mockModelLimitStore{
		err: errors.New("firestore unavailable"),
	}
	cache := newModelLimitCache(store, 60*time.Second)

	// Fail-open: should return nil (no user limits), not error.
	limits := cache.getUserLimits(context.Background(), "alice")
	if limits != nil {
		t.Fatalf("on store error: got %+v, want nil", limits)
	}
}

func TestMergedLimits_UserOverridesYAML(t *testing.T) {
	store := &mockModelLimitStore{
		limits: map[string][]*storage.ModelLimitRecord{
			"alice": {
				{UserID: "alice", ModelPrefix: "claude-opus-4", MaxDailyUSD: 10},
			},
		},
	}
	cache := newModelLimitCache(store, 60*time.Second)

	yamlLimits := []SpendLimitConfig{
		{Model: "claude-opus-4", MaxDailyUSD: 50},
		{Model: "gpt-4o", MaxDailyUSD: 25},
	}

	merged := cache.mergedLimits(context.Background(), "alice", yamlLimits)

	// Should have 2 entries: alice's claude-opus-4 ($10) + YAML gpt-4o ($25).
	if len(merged) != 2 {
		t.Fatalf("merged: got %d limits, want 2", len(merged))
	}

	// First entry should be alice's override (user limits come first).
	if merged[0].Model != "claude-opus-4" || merged[0].MaxDailyUSD != 10 {
		t.Errorf("merged[0] = {%s, %.2f}, want {claude-opus-4, 10.00}", merged[0].Model, merged[0].MaxDailyUSD)
	}

	// Second entry should be the YAML-only limit (not overridden).
	if merged[1].Model != "gpt-4o" || merged[1].MaxDailyUSD != 25 {
		t.Errorf("merged[1] = {%s, %.2f}, want {gpt-4o, 25.00}", merged[1].Model, merged[1].MaxDailyUSD)
	}
}

func TestMergedLimits_UserOnlyLimits(t *testing.T) {
	store := &mockModelLimitStore{
		limits: map[string][]*storage.ModelLimitRecord{
			"bob": {
				{UserID: "bob", ModelPrefix: "gemini-2.5-flash", MaxDailyUSD: 3},
			},
		},
	}
	cache := newModelLimitCache(store, 60*time.Second)

	// No YAML limits at all.
	merged := cache.mergedLimits(context.Background(), "bob", nil)

	if len(merged) != 1 || merged[0].Model != "gemini-2.5-flash" {
		t.Fatalf("user-only: got %+v, want [{gemini-2.5-flash, 3}]", merged)
	}
}

func TestMergedLimits_NoUserLimits_ReturnsYAML(t *testing.T) {
	store := &mockModelLimitStore{
		limits: map[string][]*storage.ModelLimitRecord{},
	}
	cache := newModelLimitCache(store, 60*time.Second)

	yamlLimits := []SpendLimitConfig{
		{Model: "claude-opus-4", MaxDailyUSD: 50},
	}

	merged := cache.mergedLimits(context.Background(), "alice", yamlLimits)

	// Should return YAML limits unchanged.
	if len(merged) != 1 || merged[0].MaxDailyUSD != 50 {
		t.Fatalf("no user limits: got %+v, want YAML", merged)
	}
}

func TestMergedLimits_StoreError_FallsBackToYAML(t *testing.T) {
	store := &mockModelLimitStore{
		err: errors.New("firestore down"),
	}
	cache := newModelLimitCache(store, 60*time.Second)

	yamlLimits := []SpendLimitConfig{
		{Model: "gpt-4o", MaxDailyUSD: 25},
	}

	merged := cache.mergedLimits(context.Background(), "alice", yamlLimits)

	// Fail-open: should return YAML limits.
	if len(merged) != 1 || merged[0].MaxDailyUSD != 25 {
		t.Fatalf("store error: got %+v, want YAML fallback", merged)
	}
}

func TestMergedLimits_CaseInsensitiveOverride(t *testing.T) {
	store := &mockModelLimitStore{
		limits: map[string][]*storage.ModelLimitRecord{
			"alice": {
				{UserID: "alice", ModelPrefix: "Claude-Opus-4", MaxDailyUSD: 10},
			},
		},
	}
	cache := newModelLimitCache(store, 60*time.Second)

	yamlLimits := []SpendLimitConfig{
		{Model: "claude-opus-4", MaxDailyUSD: 50},
	}

	merged := cache.mergedLimits(context.Background(), "alice", yamlLimits)

	// User limit (case-different) should override YAML.
	if len(merged) != 1 {
		t.Fatalf("case-insensitive: got %d limits, want 1", len(merged))
	}
	if merged[0].MaxDailyUSD != 10 {
		t.Errorf("case-insensitive: limit = %.2f, want 10.00", merged[0].MaxDailyUSD)
	}
}

func TestRecordsToLimits(t *testing.T) {
	records := []*storage.ModelLimitRecord{
		{ModelPrefix: "claude-opus-4", MaxDailyUSD: 10},
		{ModelPrefix: "gpt-4o", MaxDailyUSD: 25},
	}

	limits := recordsToLimits(records)
	if len(limits) != 2 {
		t.Fatalf("got %d limits, want 2", len(limits))
	}
	if limits[0].Model != "claude-opus-4" || limits[0].MaxDailyUSD != 10 {
		t.Errorf("limits[0] = %+v", limits[0])
	}
	if limits[1].Model != "gpt-4o" || limits[1].MaxDailyUSD != 25 {
		t.Errorf("limits[1] = %+v", limits[1])
	}
}

func TestRecordsToLimits_Nil(t *testing.T) {
	if got := recordsToLimits(nil); got != nil {
		t.Errorf("nil records: got %+v, want nil", got)
	}
}
