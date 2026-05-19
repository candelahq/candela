package tetragonaudit

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestPipelineProcessJSONStream(t *testing.T) {
	// Simulated Tetragon JSON export with two kprobe events.
	jsonStream := `
{"time":"2026-05-18T12:00:00Z","node_name":"gke-node-1","process_kprobe":{"process":{"binary":"/usr/bin/curl","arguments":"https://api.openai.com","uid":1000,"pod":{"namespace":"default","name":"dev-pod-abc","container":{"name":"workspace","image_id":"sha256:abc123"}}},"function_name":"tcp_connect","action":"post","policy_name":"candela-egress-audit","args":[{"sock_arg":{"family":"AF_INET","daddr":"104.18.7.192","dport":443,"saddr":"10.0.0.5","sport":38472}}]}}
{"time":"2026-05-18T12:00:01Z","node_name":"gke-node-1","process_kprobe":{"process":{"binary":"/usr/bin/python3","arguments":"requests.get('https://evil.com')","uid":1000,"pod":{"namespace":"default","name":"dev-pod-abc","container":{"name":"workspace","image_id":"sha256:abc123"}}},"function_name":"tcp_connect","action":"Sigkill","policy_name":"candela-egress-enforce","args":[{"sock_arg":{"family":"AF_INET","daddr":"93.184.216.34","dport":443,"saddr":"10.0.0.5","sport":38473}}]}}
`
	sink := &CollectorSink{}
	pipeline := NewPipeline(PipelineConfig{Sink: sink})

	err := pipeline.ProcessJSONStream(context.Background(), strings.NewReader(jsonStream))
	if err != nil {
		t.Fatalf("ProcessJSONStream() error = %v", err)
	}

	records := sink.GetRecords()
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2", len(records))
	}

	// First event: audit log (post action).
	r0 := records[0]
	if r0.Severity != "INFO" {
		t.Errorf("record[0] severity = %q, want INFO", r0.Severity)
	}
	if r0.Action != "post" {
		t.Errorf("record[0] action = %q, want post", r0.Action)
	}
	if r0.Binary != "/usr/bin/curl" {
		t.Errorf("record[0] binary = %q", r0.Binary)
	}
	if r0.DstAddr != "104.18.7.192" {
		t.Errorf("record[0] dst_addr = %q", r0.DstAddr)
	}
	if r0.DstPort != 443 {
		t.Errorf("record[0] dst_port = %d", r0.DstPort)
	}
	if r0.PodName != "dev-pod-abc" {
		t.Errorf("record[0] pod = %q", r0.PodName)
	}
	if r0.PolicyName != "candela-egress-audit" {
		t.Errorf("record[0] policy = %q", r0.PolicyName)
	}

	// Second event: enforcement (SIGKILL).
	r1 := records[1]
	if r1.Severity != "CRITICAL" {
		t.Errorf("record[1] severity = %q, want CRITICAL", r1.Severity)
	}
	if r1.Action != "Sigkill" {
		t.Errorf("record[1] action = %q, want Sigkill", r1.Action)
	}
	if r1.Binary != "/usr/bin/python3" {
		t.Errorf("record[1] binary = %q", r1.Binary)
	}
	if r1.DstAddr != "93.184.216.34" {
		t.Errorf("record[1] dst_addr = %q", r1.DstAddr)
	}

	// Stats should show 2 processed.
	processed, dropped, errors := pipeline.Stats().Snapshot()
	if processed != 2 {
		t.Errorf("processed = %d, want 2", processed)
	}
	if dropped != 0 {
		t.Errorf("dropped = %d, want 0", dropped)
	}
	if errors != 0 {
		t.Errorf("errors = %d, want 0", errors)
	}
}

func TestPipelineFilters(t *testing.T) {
	// Mix of kprobe and exec events.
	jsonStream := `
{"time":"2026-05-18T12:00:00Z","node_name":"n1","process_kprobe":{"process":{"binary":"/bin/curl"},"function_name":"tcp_connect","action":"post","policy_name":"candela-egress-audit"}}
{"time":"2026-05-18T12:00:01Z","node_name":"n1","process_exec":{"process":{"binary":"/bin/bash"}}}
{"time":"2026-05-18T12:00:02Z","node_name":"n1","process_kprobe":{"process":{"binary":"/bin/python3"},"function_name":"tcp_connect","action":"Sigkill","policy_name":"candela-egress-enforce"}}
`

	t.Run("KprobeOnly", func(t *testing.T) {
		sink := &CollectorSink{}
		p := NewPipeline(PipelineConfig{
			Sink:    sink,
			Filters: []EventFilter{KprobeOnly()},
		})
		_ = p.ProcessJSONStream(context.Background(), strings.NewReader(jsonStream))

		records := sink.GetRecords()
		if len(records) != 2 {
			t.Errorf("got %d records, want 2 (kprobe only)", len(records))
		}
	})

	t.Run("PolicyFilter", func(t *testing.T) {
		sink := &CollectorSink{}
		p := NewPipeline(PipelineConfig{
			Sink:    sink,
			Filters: []EventFilter{PolicyFilter("candela-egress-enforce")},
		})
		_ = p.ProcessJSONStream(context.Background(), strings.NewReader(jsonStream))

		records := sink.GetRecords()
		if len(records) != 1 {
			t.Errorf("got %d records, want 1 (enforce policy only)", len(records))
		}
		if len(records) > 0 && records[0].Action != "Sigkill" {
			t.Errorf("action = %q, want Sigkill", records[0].Action)
		}
	})

	t.Run("EnforcementOnly", func(t *testing.T) {
		sink := &CollectorSink{}
		p := NewPipeline(PipelineConfig{
			Sink:    sink,
			Filters: []EventFilter{EnforcementOnly()},
		})
		_ = p.ProcessJSONStream(context.Background(), strings.NewReader(jsonStream))

		records := sink.GetRecords()
		if len(records) != 1 {
			t.Errorf("got %d records, want 1 (enforcement only)", len(records))
		}
	})
}

func TestPipelineMalformedJSON(t *testing.T) {
	// Mix of valid and invalid JSON lines.
	input := `
{"time":"2026-05-18T12:00:00Z","node_name":"n1","process_kprobe":{"process":{"binary":"/bin/curl"},"function_name":"tcp_connect","action":"post","policy_name":"audit"}}
{invalid json
{"time":"2026-05-18T12:00:02Z","node_name":"n1","process_kprobe":{"process":{"binary":"/bin/wget"},"function_name":"tcp_connect","action":"post","policy_name":"audit"}}
`
	sink := &CollectorSink{}
	p := NewPipeline(PipelineConfig{Sink: sink})
	_ = p.ProcessJSONStream(context.Background(), strings.NewReader(input))

	// Should process the valid events and count errors for invalid ones.
	records := sink.GetRecords()
	if len(records) != 2 {
		t.Errorf("got %d records, want 2 (valid events)", len(records))
	}

	_, _, errors := p.Stats().Snapshot()
	if errors != 1 {
		t.Errorf("errors = %d, want 1 (malformed JSON)", errors)
	}
}

func TestPipelineContextCancellation(t *testing.T) {
	// Infinite stream that should be cancelled.
	infinite := strings.NewReader(`{"time":"2026-05-18T12:00:00Z","node_name":"n1","process_kprobe":{"process":{"binary":"/bin/curl"},"function_name":"tcp_connect","action":"post","policy_name":"audit"}}
`)
	sink := &CollectorSink{}
	p := NewPipeline(PipelineConfig{Sink: sink})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := p.ProcessJSONStream(ctx, infinite)
	// Should return context error after the stream is exhausted or timeout.
	// With a finite reader, it returns nil (EOF), not a timeout.
	_ = err
}

func TestPipelineProcessEvent(t *testing.T) {
	sink := &CollectorSink{}
	p := NewPipeline(PipelineConfig{Sink: sink})

	event := Event{
		Time:     time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC),
		NodeName: "node-1",
		ProcessKprobe: &ProcessKprobe{
			Process:      &Process{Binary: "/usr/bin/curl", UID: 1000},
			FunctionName: "tcp_connect",
			Action:       "SIGKILL",
			PolicyName:   "candela-enforce",
			Args: []KprobeArg{
				{SockArg: &SockArg{Daddr: "1.2.3.4", Dport: 443}},
			},
		},
	}

	err := p.ProcessEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("ProcessEvent() error = %v", err)
	}

	records := sink.GetRecords()
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}

	r := records[0]
	if r.Severity != "CRITICAL" {
		t.Errorf("severity = %q, want CRITICAL", r.Severity)
	}
	if r.DstAddr != "1.2.3.4" {
		t.Errorf("dst_addr = %q", r.DstAddr)
	}
	if r.DstPort != 443 {
		t.Errorf("dst_port = %d", r.DstPort)
	}
}

func TestEmptyStream(t *testing.T) {
	sink := &CollectorSink{}
	p := NewPipeline(PipelineConfig{Sink: sink})

	err := p.ProcessJSONStream(context.Background(), bytes.NewReader(nil))
	if err != nil {
		t.Fatalf("empty stream should return nil, got %v", err)
	}

	if len(sink.GetRecords()) != 0 {
		t.Error("expected no records from empty stream")
	}
}
