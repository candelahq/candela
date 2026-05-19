// Package tetragonaudit provides a pipeline for consuming Tetragon runtime
// security events and forwarding them to OpenTelemetry as structured audit
// logs. This enables unified observability across:
//
//   - Proxy-level LLM request telemetry (spans)
//   - Kernel-level process enforcement events (Tetragon → OTel logs)
//
// The pipeline reads Tetragon events from either:
//  1. The Tetragon gRPC export API (production: in-cluster)
//  2. A JSON log file (development/testing: tetragon export --json)
//
// Each Tetragon kprobe/tracepoint event is mapped to an OTel LogRecord with
// standardized semantic attributes for security audit trails.
package tetragonaudit

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"
)

// Event represents a Tetragon export event. This is a simplified subset of
// the full Tetragon event protobuf — we only extract fields relevant to
// the Candela audit trail.
type Event struct {
	// ProcessKprobe is populated for kprobe tracepoint events.
	ProcessKprobe *ProcessKprobe `json:"process_kprobe,omitempty"`

	// ProcessExec is populated for process execution events.
	ProcessExec *ProcessExec `json:"process_exec,omitempty"`

	// Time is the event timestamp.
	Time time.Time `json:"time"`

	// NodeName is the Kubernetes node that generated the event.
	NodeName string `json:"node_name"`
}

// ProcessKprobe represents a Tetragon kprobe event (e.g., tcp_connect intercept).
type ProcessKprobe struct {
	// Process is the process that triggered the kprobe.
	Process *Process `json:"process"`

	// FunctionName is the kernel function that was probed.
	FunctionName string `json:"function_name"`

	// Action is the enforcement action taken (e.g., "SIGKILL", "post").
	Action string `json:"action"`

	// Args contains the kprobe arguments (IP addresses, ports, etc.).
	Args []KprobeArg `json:"args"`

	// PolicyName is the TracingPolicy that matched.
	PolicyName string `json:"policy_name"`
}

// ProcessExec represents a process execution event.
type ProcessExec struct {
	Process *Process `json:"process"`
}

// Process contains process metadata from Tetragon.
type Process struct {
	// Binary is the executable path.
	Binary string `json:"binary"`

	// Arguments is the command-line arguments.
	Arguments string `json:"arguments"`

	// Pod contains Kubernetes pod metadata.
	Pod *PodInfo `json:"pod"`

	// UID is the process UID.
	UID uint32 `json:"uid"`
}

// PodInfo contains Kubernetes pod metadata from Tetragon.
type PodInfo struct {
	Namespace string         `json:"namespace"`
	Name      string         `json:"name"`
	Container *ContainerInfo `json:"container"`
}

// ContainerInfo contains container metadata.
type ContainerInfo struct {
	Name    string `json:"name"`
	ImageID string `json:"image_id"`
}

// KprobeArg represents a single kprobe argument.
type KprobeArg struct {
	SockArg *SockArg `json:"sock_arg,omitempty"`
	IntArg  *int64   `json:"int_arg,omitempty"`
}

// SockArg represents a socket argument (IP + port).
type SockArg struct {
	Family   string `json:"family"`
	Type     string `json:"type"`
	Daddr    string `json:"daddr"`
	Dport    uint32 `json:"dport"`
	Saddr    string `json:"saddr"`
	Sport    uint32 `json:"sport"`
	Protocol string `json:"protocol"`
}

// AuditRecord is the normalized audit entry emitted to the OTel pipeline.
type AuditRecord struct {
	// Timestamp of the event.
	Timestamp time.Time

	// Severity: "INFO" for allowed, "CRITICAL" for blocked.
	Severity string

	// NodeName is the Kubernetes node.
	NodeName string

	// PolicyName is the TracingPolicy that matched.
	PolicyName string

	// Action taken: "SIGKILL", "post" (log-only), etc.
	Action string

	// Process metadata.
	Binary    string
	Arguments string
	PodName   string
	Namespace string
	Container string
	UID       uint32

	// Network metadata (from kprobe args).
	DstAddr string
	DstPort uint32
	SrcAddr string
	SrcPort uint32

	// FunctionName is the probed kernel function.
	FunctionName string
}

// Sink receives normalized audit records. Implementations include OTel
// log exporters, stdout loggers, and test collectors.
type Sink interface {
	// Emit sends an audit record to the downstream pipeline.
	Emit(ctx context.Context, record AuditRecord) error
}

// Pipeline reads Tetragon events from a source and forwards them to a Sink
// as normalized AuditRecords.
type Pipeline struct {
	sink    Sink
	stats   PipelineStats
	filters []EventFilter
}

// EventFilter returns true if the event should be processed.
type EventFilter func(Event) bool

// PipelineStats tracks event processing metrics.
type PipelineStats struct {
	mu        sync.Mutex
	Processed int64
	Dropped   int64
	Errors    int64
}

// Snapshot returns a copy of the current stats.
func (s *PipelineStats) Snapshot() (processed, dropped, errors int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Processed, s.Dropped, s.Errors
}

// PipelineConfig holds configuration for the audit pipeline.
type PipelineConfig struct {
	Sink    Sink
	Filters []EventFilter
}

// NewPipeline creates an audit pipeline that normalizes Tetragon events
// and forwards them to the configured Sink.
func NewPipeline(cfg PipelineConfig) *Pipeline {
	return &Pipeline{
		sink:    cfg.Sink,
		filters: cfg.Filters,
	}
}

// Stats returns the pipeline's processing statistics.
func (p *Pipeline) Stats() *PipelineStats {
	return &p.stats
}

// ProcessJSONStream reads newline-delimited JSON Tetragon events from
// the given reader and forwards them to the sink.
// This is the primary ingestion mode for development (tetragon export --json).
func (p *Pipeline) ProcessJSONStream(ctx context.Context, r io.Reader) error {
	scanner := bufio.NewScanner(r)
	// Allow up to 1MB per line for large Tetragon events.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var event Event
		if err := json.Unmarshal(line, &event); err != nil {
			slog.Debug("tetragon-audit: failed to decode event", "error", err)
			p.stats.mu.Lock()
			p.stats.Errors++
			p.stats.mu.Unlock()
			continue
		}

		// Apply filters.
		if !p.shouldProcess(event) {
			p.stats.mu.Lock()
			p.stats.Dropped++
			p.stats.mu.Unlock()
			continue
		}

		// Normalize and emit.
		record := p.normalize(event)
		if err := p.sink.Emit(ctx, record); err != nil {
			slog.Warn("tetragon-audit: sink emit failed", "error", err)
			p.stats.mu.Lock()
			p.stats.Errors++
			p.stats.mu.Unlock()
			continue
		}

		p.stats.mu.Lock()
		p.stats.Processed++
		p.stats.mu.Unlock()
	}

	return scanner.Err()
}

// ProcessEvent processes a single pre-parsed Tetragon event.
func (p *Pipeline) ProcessEvent(ctx context.Context, event Event) error {
	if !p.shouldProcess(event) {
		p.stats.mu.Lock()
		p.stats.Dropped++
		p.stats.mu.Unlock()
		return nil
	}

	record := p.normalize(event)
	if err := p.sink.Emit(ctx, record); err != nil {
		p.stats.mu.Lock()
		p.stats.Errors++
		p.stats.mu.Unlock()
		return fmt.Errorf("emit audit record: %w", err)
	}

	p.stats.mu.Lock()
	p.stats.Processed++
	p.stats.mu.Unlock()
	return nil
}

// shouldProcess returns true if the event passes all filters.
func (p *Pipeline) shouldProcess(event Event) bool {
	for _, f := range p.filters {
		if !f(event) {
			return false
		}
	}
	return true
}

// normalize converts a Tetragon event to a normalized AuditRecord.
func (p *Pipeline) normalize(event Event) AuditRecord {
	record := AuditRecord{
		Timestamp: event.Time,
		NodeName:  event.NodeName,
		Severity:  "INFO",
	}

	if kp := event.ProcessKprobe; kp != nil {
		record.FunctionName = kp.FunctionName
		record.Action = kp.Action
		record.PolicyName = kp.PolicyName

		// SIGKILL actions are critical security events.
		if kp.Action == "SIGKILL" || kp.Action == "Sigkill" {
			record.Severity = "CRITICAL"
		}

		if kp.Process != nil {
			record.Binary = kp.Process.Binary
			record.Arguments = kp.Process.Arguments
			record.UID = kp.Process.UID
			if pod := kp.Process.Pod; pod != nil {
				record.PodName = pod.Name
				record.Namespace = pod.Namespace
				if pod.Container != nil {
					record.Container = pod.Container.Name
				}
			}
		}

		// Extract network info from kprobe args.
		for _, arg := range kp.Args {
			if arg.SockArg != nil {
				record.DstAddr = arg.SockArg.Daddr
				record.DstPort = arg.SockArg.Dport
				record.SrcAddr = arg.SockArg.Saddr
				record.SrcPort = arg.SockArg.Sport
			}
		}
	}

	if pe := event.ProcessExec; pe != nil {
		if pe.Process != nil {
			record.Binary = pe.Process.Binary
			record.Arguments = pe.Process.Arguments
			record.UID = pe.Process.UID
			if pod := pe.Process.Pod; pod != nil {
				record.PodName = pod.Name
				record.Namespace = pod.Namespace
				if pod.Container != nil {
					record.Container = pod.Container.Name
				}
			}
		}
	}

	return record
}

// ── Built-in Filters ──

// KprobeOnly filters to only kprobe events (ignoring exec, exit, etc.).
func KprobeOnly() EventFilter {
	return func(e Event) bool {
		return e.ProcessKprobe != nil
	}
}

// PolicyFilter filters to events from a specific TracingPolicy.
func PolicyFilter(policyName string) EventFilter {
	return func(e Event) bool {
		if e.ProcessKprobe == nil {
			return false
		}
		return e.ProcessKprobe.PolicyName == policyName
	}
}

// EnforcementOnly filters to events where an enforcement action was taken
// (e.g., SIGKILL), excluding log-only/post events.
func EnforcementOnly() EventFilter {
	return func(e Event) bool {
		if e.ProcessKprobe == nil {
			return false
		}
		action := e.ProcessKprobe.Action
		return action == "SIGKILL" || action == "Sigkill"
	}
}

// ── Built-in Sinks ──

// LogSink emits audit records to the structured logger.
type LogSink struct{}

func (s *LogSink) Emit(_ context.Context, r AuditRecord) error {
	slog.LogAttrs(context.Background(), severityToLevel(r.Severity),
		"tetragon audit event",
		slog.String("severity", r.Severity),
		slog.String("action", r.Action),
		slog.String("policy", r.PolicyName),
		slog.String("binary", r.Binary),
		slog.String("pod", r.PodName),
		slog.String("namespace", r.Namespace),
		slog.String("dst_addr", r.DstAddr),
		slog.Uint64("dst_port", uint64(r.DstPort)),
		slog.String("function", r.FunctionName),
		slog.String("node", r.NodeName),
	)
	return nil
}

func severityToLevel(severity string) slog.Level {
	switch severity {
	case "CRITICAL":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// CollectorSink collects audit records for testing.
type CollectorSink struct {
	mu      sync.Mutex
	Records []AuditRecord
}

func (s *CollectorSink) Emit(_ context.Context, r AuditRecord) error {
	s.mu.Lock()
	s.Records = append(s.Records, r)
	s.mu.Unlock()
	return nil
}

func (s *CollectorSink) GetRecords() []AuditRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]AuditRecord, len(s.Records))
	copy(cp, s.Records)
	return cp
}
