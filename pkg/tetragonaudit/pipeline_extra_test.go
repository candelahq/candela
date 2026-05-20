package tetragonaudit

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPipelineMultipleSinks(t *testing.T) {
	// Two independent sinks should both receive every event.
	sink1 := &CollectorSink{}
	sink2 := &CollectorSink{}

	multi := &multiSink{sinks: []Sink{sink1, sink2}}
	p := NewPipeline(PipelineConfig{Sink: multi})

	input := `{"time":"2026-05-18T12:00:00Z","node_name":"n1","process_kprobe":{"process":{"binary":"/bin/curl"},"function_name":"tcp_connect","action":"post","policy_name":"audit"}}`
	_ = p.ProcessJSONStream(context.Background(), strings.NewReader(input))

	if len(sink1.GetRecords()) != 1 {
		t.Errorf("sink1 got %d records, want 1", len(sink1.GetRecords()))
	}
	if len(sink2.GetRecords()) != 1 {
		t.Errorf("sink2 got %d records, want 1", len(sink2.GetRecords()))
	}
}

// multiSink fans out to multiple sinks.
type multiSink struct {
	sinks []Sink
}

func (m *multiSink) Emit(ctx context.Context, record AuditRecord) error {
	for _, s := range m.sinks {
		if err := s.Emit(ctx, record); err != nil {
			return err
		}
	}
	return nil
}

func TestPipelineConcurrentProcessEvent(t *testing.T) {
	sink := &CollectorSink{}
	p := NewPipeline(PipelineConfig{Sink: sink})

	const count = 20
	var wg sync.WaitGroup
	wg.Add(count)

	for i := range count {
		go func(i int) {
			defer wg.Done()
			event := Event{
				Time:     time.Now(),
				NodeName: "node-1",
				ProcessKprobe: &ProcessKprobe{
					Process:      &Process{Binary: "/bin/test"},
					FunctionName: "tcp_connect",
					Action:       "post",
					PolicyName:   "test",
				},
			}
			_ = p.ProcessEvent(context.Background(), event)
		}(i)
	}

	wg.Wait()

	records := sink.GetRecords()
	if len(records) != count {
		t.Errorf("got %d records, want %d", len(records), count)
	}

	processed, _, _ := p.Stats().Snapshot()
	if processed != int64(count) {
		t.Errorf("processed = %d, want %d", processed, count)
	}
}

func TestPipelineStatsSnapshot(t *testing.T) {
	sink := &CollectorSink{}
	p := NewPipeline(PipelineConfig{Sink: sink})

	// Process two valid, one malformed.
	input := `
{"time":"2026-05-18T12:00:00Z","node_name":"n1","process_kprobe":{"process":{"binary":"/bin/curl"},"function_name":"tcp_connect","action":"post","policy_name":"audit"}}
{bad json
{"time":"2026-05-18T12:00:01Z","node_name":"n1","process_kprobe":{"process":{"binary":"/bin/wget"},"function_name":"tcp_connect","action":"post","policy_name":"audit"}}
`
	_ = p.ProcessJSONStream(context.Background(), strings.NewReader(input))

	processed, _, errors := p.Stats().Snapshot()
	if processed != 2 {
		t.Errorf("processed = %d, want 2", processed)
	}
	if errors != 1 {
		t.Errorf("errors = %d, want 1", errors)
	}
}

func TestSIGKILLCaseInsensitive(t *testing.T) {
	// Regression: kernel/Tetragon may emit SIGKILL in different casing.
	cases := []string{"SIGKILL", "Sigkill", "sigkill", "SigKill"}

	for _, action := range cases {
		t.Run("normalize_"+action, func(t *testing.T) {
			sink := &CollectorSink{}
			p := NewPipeline(PipelineConfig{Sink: sink})
			event := Event{
				NodeName: "n1",
				ProcessKprobe: &ProcessKprobe{
					Process:      &Process{Binary: "/bin/kill"},
					FunctionName: "security_task_kill",
					Action:       action,
					PolicyName:   "enforce",
				},
			}
			_ = p.ProcessEvent(context.Background(), event)
			records := sink.GetRecords()
			if len(records) != 1 {
				t.Fatalf("got %d records, want 1", len(records))
			}
			if records[0].Severity != "CRITICAL" {
				t.Errorf("action=%q: severity=%q, want CRITICAL", action, records[0].Severity)
			}
		})

		t.Run("enforcementFilter_"+action, func(t *testing.T) {
			filter := EnforcementOnly()
			event := Event{
				ProcessKprobe: &ProcessKprobe{Action: action},
			}
			if !filter(event) {
				t.Errorf("EnforcementOnly() rejected action=%q, want accepted", action)
			}
		})
	}
}
