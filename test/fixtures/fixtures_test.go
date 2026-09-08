package fixtures_test

import (
	"encoding/json"
	"os"
	"testing"
)

type userDocument struct {
	Name       string `json:"name"`
	CreateTime string `json:"createTime"`
	UpdateTime string `json:"updateTime"`
	Fields     struct {
		ID struct {
			StringValue string `json:"stringValue"`
		} `json:"id"`
		Email struct {
			StringValue string `json:"stringValue"`
		} `json:"email"`
		Role struct {
			StringValue string `json:"stringValue"`
		} `json:"role"`
		Status struct {
			StringValue string `json:"stringValue"`
		} `json:"status"`
		CreatedAt struct {
			TimestampValue string `json:"timestampValue"`
		} `json:"created_at"`
	} `json:"fields"`
}

type userExport struct {
	Documents []userDocument `json:"documents"`
}

func TestUsersFixture(t *testing.T) {
	data, err := os.ReadFile("users.json")
	if err != nil {
		t.Fatalf("failed to read test/fixtures/users.json: %v", err)
	}

	var export userExport
	if err := json.Unmarshal(data, &export); err != nil {
		t.Fatalf("failed to parse test/fixtures/users.json: %v", err)
	}

	if len(export.Documents) == 0 {
		t.Fatalf("expected at least one user document in fixture, got 0")
	}

	for i, doc := range export.Documents {
		if doc.Name == "" {
			t.Errorf("document %d: missing name", i)
		}
		if doc.Fields.ID.StringValue == "" {
			t.Errorf("document %d: missing id", i)
		}
		if doc.Fields.Email.StringValue == "" {
			t.Errorf("document %d: missing email", i)
		}
		if doc.Fields.Role.StringValue == "" {
			t.Errorf("document %d: missing role", i)
		}
		if doc.Fields.Status.StringValue == "" {
			t.Errorf("document %d: missing status", i)
		}
		if doc.Fields.CreatedAt.TimestampValue == "" {
			t.Errorf("document %d: missing created_at", i)
		}
		if doc.CreateTime == "" {
			t.Errorf("document %d: missing createTime", i)
		}
		if doc.UpdateTime == "" {
			t.Errorf("document %d: missing updateTime", i)
		}
	}
}
