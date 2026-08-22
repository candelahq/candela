package taskspend

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/candelahq/candela/pkg/storage"
)

// Handler returns an http.HandlerFunc that serves GET /api/v1/task-spend/{taskID}.
// It reads from the Cache (which read-throughs to Firestore with TTL).
func Handler(cache *Cache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		taskID := extractTaskID(r.URL.Path)
		if taskID == "" {
			http.Error(w, `{"error":"task_id is required"}`, http.StatusBadRequest)
			return
		}

		snap, err := cache.Get(r.Context(), taskID)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				http.Error(w, `{"error":"task budget not found"}`, http.StatusNotFound)
				return
			}
			slog.Error("taskspend: handler error", "task_id", taskID, "error", err)
			http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-cache")
		if err := json.NewEncoder(w).Encode(snap); err != nil {
			slog.Error("taskspend: failed to encode response", "error", err)
		}
	}
}

// extractTaskID pulls the task ID from a URL path like
// /api/v1/task-spend/{taskID}
func extractTaskID(path string) string {
	const prefix = "/api/v1/task-spend/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	id := strings.TrimPrefix(path, prefix)
	// Strip any trailing slash.
	id = strings.TrimRight(id, "/")
	return id
}
