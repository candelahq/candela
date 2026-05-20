// Package hubbleaudit defines stub interfaces for Hubble flow correlation.
// This prepares the codebase for future integration with Cilium Hubble's
// network flow observability, enabling unified security+network telemetry.
package hubbleaudit

import (
	"context"
	"time"
)

// Flow represents a Hubble network flow observation.
// This mirrors the core fields from the Hubble flow.Flow protobuf message,
// abstracted to avoid a hard dependency on the Hubble API.
type Flow struct {
	// Time is the timestamp of the flow observation.
	Time time.Time `json:"time"`

	// Source identifies the originating workload.
	Source Endpoint `json:"source"`

	// Destination identifies the target workload.
	Destination Endpoint `json:"destination"`

	// Verdict is the policy decision (FORWARDED, DROPPED, ERROR).
	Verdict string `json:"verdict"`

	// Type is the flow type (L3_L4, L7, etc.).
	Type string `json:"type"`

	// Summary is a human-readable one-line flow summary.
	Summary string `json:"summary"`

	// IsReply indicates whether this is a reply flow.
	IsReply bool `json:"is_reply"`

	// TraceObservationPoint is where in the datapath the flow was observed.
	TraceObservationPoint string `json:"trace_observation_point,omitempty"`
}

// Endpoint represents a network endpoint in a Hubble flow.
type Endpoint struct {
	// Namespace is the Kubernetes namespace.
	Namespace string `json:"namespace,omitempty"`

	// PodName is the Kubernetes pod name.
	PodName string `json:"pod_name,omitempty"`

	// Labels are Cilium identity labels.
	Labels []string `json:"labels,omitempty"`

	// IP is the endpoint IP address.
	IP string `json:"ip,omitempty"`

	// Port is the L4 port number.
	Port uint32 `json:"port,omitempty"`

	// Identity is the Cilium numeric identity.
	Identity uint32 `json:"identity,omitempty"`
}

// FlowSource provides Hubble flow events.
type FlowSource interface {
	// Observe starts streaming flows matching the given filter.
	// The returned channel is closed when the context is cancelled or
	// the source encounters a fatal error.
	Observe(ctx context.Context, filter FlowFilter) (<-chan Flow, error)
}

// FlowFilter specifies criteria for filtering Hubble flows.
type FlowFilter struct {
	// Namespace limits flows to a specific Kubernetes namespace.
	Namespace string `json:"namespace,omitempty"`

	// PodName limits flows to a specific pod.
	PodName string `json:"pod_name,omitempty"`

	// Verdicts limits flows to specific verdicts.
	Verdicts []string `json:"verdicts,omitempty"`

	// Since limits flows to those observed after this time.
	Since *time.Time `json:"since,omitempty"`
}

// Correlator matches Tetragon enforcement events to Hubble network flows,
// providing a unified view of kernel security decisions and network behavior.
type Correlator interface {
	// Correlate attempts to find Hubble flows related to an enforcement event.
	// The key is typically "src_ip:src_port→dst_ip:dst_port".
	Correlate(ctx context.Context, key string, window time.Duration) ([]Flow, error)
}
