package tetragonaudit

import (
	"context"
	"errors"
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

func TestPipelineConcurrentStats(t *testing.T) {
	sink := &intermittentErrSink{failEvery: 3} // fails every 3rd emit
	filter := func(e Event) bool {
		// drop events with function_name "drop_me"
		return e.ProcessKprobe == nil || e.ProcessKprobe.FunctionName != "drop_me"
	}
	p := NewPipeline(PipelineConfig{
		Sink:    sink,
		Filters: []EventFilter{filter},
	})

	const numWorkers = 10
	const eventsPerWorker = 30
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for w := range numWorkers {
		go func(workerID int) {
			defer wg.Done()
			for i := range eventsPerWorker {
				fn := "tcp_connect"
				if (workerID*eventsPerWorker+i)%3 == 1 {
					fn = "drop_me"
				}
				event := Event{
					Time:     time.Now(),
					NodeName: "node-1",
					ProcessKprobe: &ProcessKprobe{
						Process:      &Process{Binary: "/bin/test"},
						FunctionName: fn,
						Action:       "post",
						PolicyName:   "test",
					},
				}
				_ = p.ProcessEvent(context.Background(), event)
			}
		}(w)
	}

	wg.Wait()

	processed, dropped, errors := p.Stats().Snapshot()
	total := processed + dropped + errors
	expectedTotal := int64(numWorkers * eventsPerWorker)
	if total != expectedTotal {
		t.Errorf("total stats = %d (%d processed, %d dropped, %d errors), want %d",
			total, processed, dropped, errors, expectedTotal)
	}
	if dropped != 100 {
		t.Errorf("dropped = %d, want 100", dropped)
	}
	if processed != 134 {
		t.Errorf("processed = %d, want 134", processed)
	}
	if errors != 66 {
		t.Errorf("errors = %d, want 66", errors)
	}
}

// intermittentErrSink fails every Nth emit.
type intermittentErrSink struct {
	mu        sync.Mutex
	count     int
	failEvery int
}

func (s *intermittentErrSink) Emit(_ context.Context, _ AuditRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.count++
	if s.failEvery > 0 && s.count%s.failEvery == 0 {
		return errors.New("simulated sink error")
	}
	return nil
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
