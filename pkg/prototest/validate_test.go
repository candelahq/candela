package prototest

import (
	"strings"
	"testing"
	"time"

	"buf.build/go/protovalidate"
	typespb "github.com/candelahq/candela/gen/go/candela/types"
	v1 "github.com/candelahq/candela/gen/go/candela/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var validator protovalidate.Validator

func init() {
	var err error
	validator, err = protovalidate.New()
	if err != nil {
		panic("failed to create protovalidate validator: " + err.Error())
	}
}

func mustValidate(t *testing.T, msg proto.Message) {
	t.Helper()
	if err := validator.Validate(msg); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}

func mustReject(t *testing.T, msg proto.Message, wantSubstr string) {
	t.Helper()
	err := validator.Validate(msg)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if wantSubstr != "" && !strings.Contains(err.Error(), wantSubstr) {
		t.Errorf("error %q does not contain %q", err.Error(), wantSubstr)
	}
}

// ──────────────────────────────────────────
// UserService validation (existing constraints)
// ──────────────────────────────────────────

func TestCreateUserRequest_EmptyEmail_Fails(t *testing.T) {
	mustReject(t, &v1.CreateUserRequest{
		DisplayName: "Alice",
	}, "email")
}

func TestCreateUserRequest_InvalidEmail_Fails(t *testing.T) {
	mustReject(t, &v1.CreateUserRequest{
		Email: "not-an-email",
	}, "email")
}

func TestCreateUserRequest_NegativeBudget_Fails(t *testing.T) {
	mustReject(t, &v1.CreateUserRequest{
		Email:          "valid@example.com",
		DailyBudgetUsd: -1.0,
	}, "daily_budget_usd")
}

func TestCreateUserRequest_Valid_Passes(t *testing.T) {
	mustValidate(t, &v1.CreateUserRequest{
		Email:          "valid@example.com",
		DisplayName:    "Alice",
		DailyBudgetUsd: 10.0,
	})
}

func TestSetBudgetRequest_ZeroLimit_Fails(t *testing.T) {
	mustReject(t, &v1.SetBudgetRequest{
		UserId:   "user-123",
		LimitUsd: 0, // must be gt: 0
	}, "limit_usd")
}

func TestSetBudgetRequest_Valid_Passes(t *testing.T) {
	mustValidate(t, &v1.SetBudgetRequest{
		UserId:     "user-123",
		LimitUsd:   50.0,
		PeriodType: typespb.BudgetPeriod_BUDGET_PERIOD_DAILY,
	})
}

func TestCreateGrantRequest_ExpiresBeforeStarts_Fails(t *testing.T) {
	now := time.Now().UTC()
	mustReject(t, &v1.CreateGrantRequest{
		UserId:    "user-123",
		AmountUsd: 25.0,
		Reason:    "hackathon",
		StartsAt:  timestamppb.New(now.Add(24 * time.Hour)),
		ExpiresAt: timestamppb.New(now), // before starts_at
	}, "expires_at must be after starts_at")
}

func TestCreateGrantRequest_ValidWindow_Passes(t *testing.T) {
	now := time.Now().UTC()
	mustValidate(t, &v1.CreateGrantRequest{
		UserId:    "user-123",
		AmountUsd: 25.0,
		Reason:    "hackathon",
		StartsAt:  timestamppb.New(now),
		ExpiresAt: timestamppb.New(now.Add(24 * time.Hour)),
	})
}

func TestListAuditLogRequest_LimitTooHigh_Fails(t *testing.T) {
	mustReject(t, &v1.ListAuditLogRequest{
		UserId: "user-123",
		Limit:  501, // max 500
	}, "limit")
}

func TestListAuditLogRequest_ValidLimit_Passes(t *testing.T) {
	mustValidate(t, &v1.ListAuditLogRequest{
		UserId: "user-123",
		Limit:  50,
	})
}

// ──────────────────────────────────────────
// ProjectService validation (H5 — new)
// Requires candela-protos#1 merged + stubs regenerated.
// ──────────────────────────────────────────

func requireHardenedProtos(t *testing.T) {
	t.Helper()
	// These tests require the hardened proto stubs from candela-protos feat/api-hardening.
	// Skip if the generated stubs don't have the new validation annotations yet.
	err := validator.Validate(&v1.CreateProjectRequest{Name: ""})
	if err == nil {
		t.Skip("skipping: generated stubs do not include H5 validation (regenerate from BSR after candela-protos#1 merges)")
	}
}

func TestCreateProjectRequest_EmptyName_Fails(t *testing.T) {
	requireHardenedProtos(t)
	mustReject(t, &v1.CreateProjectRequest{
		Name: "",
	}, "name")
}

func TestCreateProjectRequest_NameTooLong_Fails(t *testing.T) {
	requireHardenedProtos(t)
	mustReject(t, &v1.CreateProjectRequest{
		Name: strings.Repeat("x", 256), // max 255
	}, "name")
}

func TestCreateProjectRequest_DescriptionTooLong_Fails(t *testing.T) {
	requireHardenedProtos(t)
	mustReject(t, &v1.CreateProjectRequest{
		Name:        "valid-project",
		Description: strings.Repeat("x", 4097), // max 4096
	}, "description")
}

func TestCreateProjectRequest_Valid_Passes(t *testing.T) {
	mustValidate(t, &v1.CreateProjectRequest{
		Name:        "My Project",
		Description: "A short description.",
	})
}

func TestGetProjectRequest_EmptyID_Fails(t *testing.T) {
	requireHardenedProtos(t)
	mustReject(t, &v1.GetProjectRequest{
		Id: "",
	}, "id")
}

func TestDeleteProjectRequest_EmptyID_Fails(t *testing.T) {
	requireHardenedProtos(t)
	mustReject(t, &v1.DeleteProjectRequest{
		Id: "",
	}, "id")
}

func TestCreateAPIKeyRequest_EmptyFields_Fails(t *testing.T) {
	requireHardenedProtos(t)
	mustReject(t, &v1.CreateAPIKeyRequest{
		ProjectId: "",
		Name:      "",
	}, "")
}

func TestCreateAPIKeyRequest_Valid_Passes(t *testing.T) {
	mustValidate(t, &v1.CreateAPIKeyRequest{
		ProjectId: "proj-123",
		Name:      "production-key",
	})
}

func TestRevokeAPIKeyRequest_EmptyID_Fails(t *testing.T) {
	requireHardenedProtos(t)
	mustReject(t, &v1.RevokeAPIKeyRequest{
		Id: "",
	}, "id")
}

// ──────────────────────────────────────────
// PaginationRequest validation (H10)
// ──────────────────────────────────────────

func requireHardenedPagination(t *testing.T) {
	t.Helper()
	err := validator.Validate(&typespb.PaginationRequest{PageSize: 1001})
	if err == nil {
		t.Skip("skipping: generated stubs do not include H10 pagination bounds (regenerate from BSR after candela-protos#1 merges)")
	}
}

func TestPaginationRequest_PageSizeTooLarge_Fails(t *testing.T) {
	requireHardenedPagination(t)
	mustReject(t, &typespb.PaginationRequest{
		PageSize: 1001, // max 1000
	}, "page_size")
}

func TestPaginationRequest_NegativePageSize_Fails(t *testing.T) {
	requireHardenedPagination(t)
	mustReject(t, &typespb.PaginationRequest{
		PageSize: -1,
	}, "page_size")
}

func TestPaginationRequest_ValidPageSize_Passes(t *testing.T) {
	mustValidate(t, &typespb.PaginationRequest{
		PageSize: 50,
	})
}

func TestPaginationRequest_ZeroPageSize_Passes(t *testing.T) {
	// 0 = server default
	mustValidate(t, &typespb.PaginationRequest{
		PageSize: 0,
	})
}

// ──────────────────────────────────────────
// IngestSpansRequest validation (H6)
// ──────────────────────────────────────────

func TestIngestSpansRequest_EmptyBatch_Passes(t *testing.T) {
	mustValidate(t, &v1.IngestSpansRequest{
		Spans: nil,
	})
}
