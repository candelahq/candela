package hubbleaudit

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// CorrelatorConfig configures the WindowCorrelator.
type CorrelatorConfig struct {
	// MaxFlows is the maximum number of flows in the ring buffer (default: 10000).
	MaxFlows int

	// TTL is how long flows are retained before eviction (default: 5m).
	TTL time.Duration
}

func (c CorrelatorConfig) withDefaults() CorrelatorConfig {
	if c.MaxFlows <= 0 {
		c.MaxFlows = 10000
	}
	if c.TTL <= 0 {
		c.TTL = 5 * time.Minute
	}
	return c
}

// timestampedFlow wraps a Flow with its ingestion timestamp for TTL checks.
type timestampedFlow struct {
	flow       Flow
	ingestedAt time.Time
}

// WindowCorrelator implements the Correlator interface using an in-memory
// ring buffer of recent Hubble flows. Flows are indexed by pod identity
// (namespace/pod-name) and by network tuple (src_ip:src_port→dst_ip:dst_port)
// for fast lookup during correlation.
//
// Thread-safe: concurrent Ingest and Correlate calls are supported via RWMutex.
type WindowCorrelator struct {
	cfg CorrelatorConfig

	mu    sync.RWMutex
	ring  []timestampedFlow
	head  int // next write position
	count int // number of valid entries (≤ len(ring))

	// podIndex maps "namespace/pod-name" → indices into ring buffer.
	podIndex map[string][]int

	// tupleIndex maps "src_ip:src_port→dst_ip:dst_port" → indices into ring buffer.
	tupleIndex map[string][]int
}

// NewWindowCorrelator creates a correlator with the given configuration.
func NewWindowCorrelator(cfg CorrelatorConfig) *WindowCorrelator {
	cfg = cfg.withDefaults()
	return &WindowCorrelator{
		cfg:        cfg,
		ring:       make([]timestampedFlow, cfg.MaxFlows),
		podIndex:   make(map[string][]int),
		tupleIndex: make(map[string][]int),
	}
}

// Ingest adds a flow to the correlator's ring buffer.
// Old flows at the write position are evicted from indices.
func (c *WindowCorrelator) Ingest(flow Flow) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict old entry at this position if ring is full.
	if c.count == len(c.ring) {
		c.evictAt(c.head)
	}

	c.ring[c.head] = timestampedFlow{
		flow:       flow,
		ingestedAt: time.Now(),
	}

	// Index by source pod.
	srcKey := podKey(flow.Source)
	if srcKey != "" {
		c.podIndex[srcKey] = append(c.podIndex[srcKey], c.head)
	}
	// Index by destination pod (skip if same as source to avoid duplicates).
	dstKey := podKey(flow.Destination)
	if dstKey != "" && dstKey != srcKey {
		c.podIndex[dstKey] = append(c.podIndex[dstKey], c.head)
	}
	// Index by network tuple.
	if key := tupleKey(flow); key != "" {
		c.tupleIndex[key] = append(c.tupleIndex[key], c.head)
	}

	if c.count < len(c.ring) {
		c.count++
	}
	c.head = (c.head + 1) % len(c.ring)
}

// evictAt removes index entries for the flow at position pos.
func (c *WindowCorrelator) evictAt(pos int) {
	old := c.ring[pos].flow

	srcKey := podKey(old.Source)
	if srcKey != "" {
		c.podIndex[srcKey] = removeIndex(c.podIndex[srcKey], pos)
		if len(c.podIndex[srcKey]) == 0 {
			delete(c.podIndex, srcKey)
		}
	}
	dstKey := podKey(old.Destination)
	if dstKey != "" && dstKey != srcKey {
		c.podIndex[dstKey] = removeIndex(c.podIndex[dstKey], pos)
		if len(c.podIndex[dstKey]) == 0 {
			delete(c.podIndex, dstKey)
		}
	}
	if key := tupleKey(old); key != "" {
		c.tupleIndex[key] = removeIndex(c.tupleIndex[key], pos)
		if len(c.tupleIndex[key]) == 0 {
			delete(c.tupleIndex, key)
		}
	}
}

// Correlate returns flows matching the given key within the time window.
// The key can be either "namespace/pod-name" (pod identity) or
// "src_ip:src_port→dst_ip:dst_port" (network tuple).
func (c *WindowCorrelator) Correlate(_ context.Context, key string, window time.Duration) ([]Flow, error) {
	if key == "" {
		return nil, fmt.Errorf("hubbleaudit: correlation key is required")
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	cutoff := time.Now().Add(-window)
	var result []Flow

	// Try pod index first, then tuple index.
	indices := c.podIndex[key]
	if len(indices) == 0 {
		indices = c.tupleIndex[key]
	}

	for _, idx := range indices {
		entry := c.ring[idx]
		if entry.ingestedAt.Before(cutoff) {
			continue // expired
		}
		if entry.flow.Time.Before(cutoff) {
			continue // flow itself is outside the window
		}
		result = append(result, copyFlow(entry.flow))
	}

	return result, nil
}

// Stats returns current correlator statistics.
type CorrelatorStats struct {
	FlowCount    int `json:"flow_count"`
	PodKeys      int `json:"pod_keys"`
	TupleKeys    int `json:"tuple_keys"`
	RingCapacity int `json:"ring_capacity"`
}

// Stats returns a snapshot of correlator state.
func (c *WindowCorrelator) Stats() CorrelatorStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return CorrelatorStats{
		FlowCount:    c.count,
		PodKeys:      len(c.podIndex),
		TupleKeys:    len(c.tupleIndex),
		RingCapacity: len(c.ring),
	}
}

// podKey returns "namespace/pod-name" for an endpoint, or "" if not available.
func podKey(ep Endpoint) string {
	if ep.Namespace == "" || ep.PodName == "" {
		return ""
	}
	return ep.Namespace + "/" + ep.PodName
}

// tupleKey returns "src_ip:src_port→dst_ip:dst_port" for a flow, or "" if
// source/dest IPs are not available.
func tupleKey(f Flow) string {
	if f.Source.IP == "" || f.Destination.IP == "" {
		return ""
	}
	return fmt.Sprintf("%s:%d→%s:%d",
		f.Source.IP, f.Source.Port,
		f.Destination.IP, f.Destination.Port)
}

// removeIndex removes val from a slice of ints (preserving order).
func removeIndex(s []int, val int) []int {
	for i, v := range s {
		if v == val {
			return append(s[:i], s[i+1:]...)
		}
	}
	return s
}

// copyEndpoint returns a shallow copy of an Endpoint with its labels slice
// duplicated to prevent callers from mutating internal state.
func copyEndpoint(ep Endpoint) Endpoint {
	if len(ep.Labels) > 0 {
		labels := make([]string, len(ep.Labels))
		copy(labels, ep.Labels)
		ep.Labels = labels
	}
	return ep
}

// copyFlow returns a defensive copy of a Flow, duplicating label slices
// in both endpoints so callers cannot mutate the correlator's ring buffer.
func copyFlow(f Flow) Flow {
	f.Source = copyEndpoint(f.Source)
	f.Destination = copyEndpoint(f.Destination)
	return f
}
