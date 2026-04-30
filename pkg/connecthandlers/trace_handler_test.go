package connecthandlers

import (
	"testing"

	"github.com/candelahq/candela/pkg/storage"
)

func TestTraceUserID(t *testing.T) {
	tests := []struct {
		name   string
		trace  *storage.Trace
		wantID string
	}{
		{
			name: "root span has user_id",
			trace: &storage.Trace{
				TraceID: "t1",
				Spans: []storage.Span{
					{SpanID: "root", ParentSpanID: "", UserID: "alice@example.com"},
					{SpanID: "child", ParentSpanID: "root", UserID: "alice@example.com"},
				},
			},
			wantID: "alice@example.com",
		},
		{
			name: "root span empty, child has user_id",
			trace: &storage.Trace{
				TraceID: "t2",
				Spans: []storage.Span{
					{SpanID: "root", ParentSpanID: ""},
					{SpanID: "child", ParentSpanID: "root", UserID: "bob@example.com"},
				},
			},
			wantID: "bob@example.com",
		},
		{
			name: "no spans have user_id (legacy)",
			trace: &storage.Trace{
				TraceID: "t3",
				Spans: []storage.Span{
					{SpanID: "root", ParentSpanID: ""},
					{SpanID: "child", ParentSpanID: "root"},
				},
			},
			wantID: "",
		},
		{
			name: "empty trace",
			trace: &storage.Trace{
				TraceID: "t4",
				Spans:   nil,
			},
			wantID: "",
		},
		{
			name: "root has different user than child — root wins",
			trace: &storage.Trace{
				TraceID: "t5",
				Spans: []storage.Span{
					{SpanID: "root", ParentSpanID: "", UserID: "alice@example.com"},
					{SpanID: "child", ParentSpanID: "root", UserID: "bob@example.com"},
				},
			},
			wantID: "alice@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := traceUserID(tt.trace)
			if got != tt.wantID {
				t.Errorf("traceUserID() = %q, want %q", got, tt.wantID)
			}
		})
	}
}
