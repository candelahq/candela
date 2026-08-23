// Package modellimits provides a REST API handler for per-user per-model
// daily spend limits (#721). Admin-only CRUD operations.
//
// Endpoints:
//
//	PUT    /api/v1/users/{userID}/model-limits/{modelPrefix}  — set/update a limit
//	GET    /api/v1/users/{userID}/model-limits                — list all limits
//	DELETE /api/v1/users/{userID}/model-limits/{modelPrefix}  — remove a limit
package modellimits

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/candelahq/candela/pkg/auth"
	"github.com/candelahq/candela/pkg/storage"
)

// Store is the minimal interface for model limit persistence.
type Store interface {
	SetModelLimit(ctx context.Context, limit *storage.ModelLimitRecord) error
	GetModelLimits(ctx context.Context, userID string) ([]*storage.ModelLimitRecord, error)
	DeleteModelLimit(ctx context.Context, userID, modelPrefix string) error
}

// UserLookup is the minimal interface for admin role checks.
type UserLookup interface {
	GetUser(ctx context.Context, id string) (*storage.UserRecord, error)
}

// setRequest is the JSON body for PUT requests.
type setRequest struct {
	MaxDailyUSD float64 `json:"max_daily_usd"`
}

// Handler returns an http.HandlerFunc that routes model limit CRUD.
//
// Authorization: admin only. Unauthenticated → 401. Non-admin → 403.
func Handler(store Store, users UserLookup) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// ── Authentication ──
		caller := auth.FromContext(r.Context())
		if caller == nil {
			jsonError(w, "authentication required", http.StatusUnauthorized)
			return
		}

		// ── Authorization: admin only ──
		if !isAdmin(r.Context(), caller.EffectiveID(), users) {
			jsonError(w, "admin access required", http.StatusForbidden)
			return
		}

		// Parse path: /api/v1/users/{userID}/model-limits[/{modelPrefix}]
		userID, modelPrefix := parsePath(r.URL.Path)
		if userID == "" {
			jsonError(w, "user_id is required", http.StatusBadRequest)
			return
		}

		switch r.Method {
		case http.MethodPut:
			handleSet(w, r, store, userID, modelPrefix)
		case http.MethodGet:
			if modelPrefix != "" {
				jsonError(w, "model_prefix not accepted on GET — use GET /api/v1/users/{userID}/model-limits", http.StatusBadRequest)
				return
			}
			handleList(w, r, store, userID)
		case http.MethodDelete:
			handleDelete(w, r, store, userID, modelPrefix)
		default:
			jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func handleSet(w http.ResponseWriter, r *http.Request, store Store, userID, modelPrefix string) {
	if modelPrefix == "" {
		jsonError(w, "model_prefix is required in URL path", http.StatusBadRequest)
		return
	}

	var req setRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.MaxDailyUSD <= 0 {
		jsonError(w, "max_daily_usd must be > 0", http.StatusBadRequest)
		return
	}

	limit := &storage.ModelLimitRecord{
		UserID:      userID,
		ModelPrefix: modelPrefix,
		MaxDailyUSD: req.MaxDailyUSD,
	}
	if err := store.SetModelLimit(r.Context(), limit); err != nil {
		slog.Error("modellimits: set failed", "user_id", userID,
			"model_prefix", modelPrefix, "error", err)
		jsonError(w, "internal server error", http.StatusInternalServerError)
		return
	}

	slog.Info("model limit set",
		"user_id", userID, "model_prefix", modelPrefix,
		"max_daily_usd", req.MaxDailyUSD,
		"actor", auth.EmailFromContext(r.Context()))

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(limit)
}

func handleList(w http.ResponseWriter, r *http.Request, store Store, userID string) {
	limits, err := store.GetModelLimits(r.Context(), userID)
	if err != nil {
		slog.Error("modellimits: list failed", "user_id", userID, "error", err)
		jsonError(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"user_id": userID,
		"limits":  limits,
	})
}

func handleDelete(w http.ResponseWriter, r *http.Request, store Store, userID, modelPrefix string) {
	if modelPrefix == "" {
		jsonError(w, "model_prefix is required in URL path", http.StatusBadRequest)
		return
	}

	if err := store.DeleteModelLimit(r.Context(), userID, modelPrefix); err != nil {
		slog.Error("modellimits: delete failed", "user_id", userID,
			"model_prefix", modelPrefix, "error", err)
		jsonError(w, "internal server error", http.StatusInternalServerError)
		return
	}

	slog.Info("model limit deleted",
		"user_id", userID, "model_prefix", modelPrefix,
		"actor", auth.EmailFromContext(r.Context()))

	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprint(w, `{"ok":true}`)
}

// parsePath extracts userID and optional modelPrefix from the URL path.
// Expected: /api/v1/users/{userID}/model-limits[/{modelPrefix}]
//
// Requires "model-limits" as an exact path segment — rejects "model-limits-extra".
func parsePath(path string) (userID, modelPrefix string) {
	const prefix = "/api/v1/users/"
	if !strings.HasPrefix(path, prefix) {
		return "", ""
	}
	rest := strings.TrimPrefix(path, prefix)

	// Split remaining path into segments: [userID, "model-limits", modelPrefix?]
	segments := strings.Split(strings.Trim(rest, "/"), "/")
	if len(segments) < 2 {
		return "", ""
	}

	// segments[0] = userID, segments[1] must be exactly "model-limits"
	if segments[1] != "model-limits" {
		return "", ""
	}

	userID = segments[0]
	if len(segments) >= 3 && segments[2] != "" {
		modelPrefix = segments[2]
	}

	return userID, modelPrefix
}

// isAdmin checks if the caller has admin role.
func isAdmin(ctx context.Context, callerID string, users UserLookup) bool {
	if users == nil {
		return false
	}
	record, err := users.GetUser(ctx, callerID)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			slog.Error("modellimits: admin lookup failed", "error", err)
		}
		return false
	}
	return record != nil && record.Role == storage.RoleAdmin
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
