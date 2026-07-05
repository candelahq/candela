package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestSlogLogger_OutputFields(t *testing.T) {
	// Capture slog output.
	var buf bytes.Buffer
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	defer slog.SetDefault(oldLogger)

	l := NewSlogLogger()
	l.Log(context.Background(), Event{
		ActorEmail: "admin@test.com",
		ActorID:    "uid-789",
		Service:    "UserService",
		Method:     "DeleteUser",
		Procedure:  "/candela.v1.UserService/DeleteUser",
		StatusCode: "ok",
		Error:      "",
	})

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse slog output: %v\nraw: %s", err, buf.String())
	}

	want := map[string]string{
		"actor_email": "admin@test.com",
		"actor_id":    "uid-789",
		"service":     "UserService",
		"method":      "DeleteUser",
		"procedure":   "/candela.v1.UserService/DeleteUser",
		"status":      "ok",
		"error":       "",
	}
	for key, expected := range want {
		got, ok := entry[key].(string)
		if !ok {
			t.Errorf("key %q not found or not a string in output", key)
			continue
		}
		if got != expected {
			t.Errorf("%s = %q, want %q", key, got, expected)
		}
	}

	// Verify it's an INFO-level audit message.
	if level, ok := entry["level"].(string); !ok || level != "INFO" {
		t.Errorf("level = %v, want %q", entry["level"], "INFO")
	}
	if msg, ok := entry["msg"].(string); !ok || msg != "audit" {
		t.Errorf("msg = %v, want %q", entry["msg"], "audit")
	}
}

func TestSlogLogger_ErrorEvent(t *testing.T) {
	var buf bytes.Buffer
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	defer slog.SetDefault(oldLogger)

	l := NewSlogLogger()
	l.Log(context.Background(), Event{
		ActorEmail: "hacker@evil.com",
		Service:    "UserService",
		Method:     "DeleteUser",
		Procedure:  "/candela.v1.UserService/DeleteUser",
		StatusCode: "permission_denied",
		Error:      "not allowed",
	})

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse slog output: %v", err)
	}

	if got := entry["status"]; got != "permission_denied" {
		t.Errorf("status = %v, want %q", got, "permission_denied")
	}
	if got := entry["error"]; got != "not allowed" {
		t.Errorf("error = %v, want %q", got, "not allowed")
	}
}
