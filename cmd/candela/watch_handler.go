package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/candelahq/candela/pkg/processor"
)

type watchHandler struct {
	proc *processor.SpanProcessor
}

func newWatchHandler(proc *processor.SpanProcessor) http.Handler {
	return &watchHandler{proc: proc}
}

func (h *watchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	filter := processor.WatchFilter{
		ProjectID: r.URL.Query().Get("project"),
		Model:     r.URL.Query().Get("model"),
		Provider:  r.URL.Query().Get("provider"),
	}

	if h.proc == nil {
		http.Error(w, "trace processing not available", http.StatusServiceUnavailable)
		return
	}

	ch, cleanup := h.proc.Subscribe(filter)
	defer cleanup()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case trace, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(trace)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}
