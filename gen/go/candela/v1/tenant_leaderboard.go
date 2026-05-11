// Package candelav1 — hand-authored extension for multitenant leaderboard types.
//
// These types extend the generated proto types without modifying the BSR-managed
// proto files. They will be upstreamed to the proto definition in a follow-up PR.
//
// NOTE: Do NOT add protoimpl machinery here — these are plain Go structs used
// directly with the Connect handler extension below.

package candelav1

import (
	"github.com/candelahq/candela/gen/go/candela/types"
)

// GetTenantLeaderboardRequest is the request type for the tenant cost leaderboard.
// Admin-only: returns all tenants ranked by LLM cost for the given project/period.
type GetTenantLeaderboardRequest struct {
	// project_id scopes the query to a specific Candela project.
	ProjectId string `json:"project_id,omitempty"`

	// time_range filters to spans within the given window.
	// Defaults to the last 30 days if unset.
	TimeRange *types.TimeRange `json:"time_range,omitempty"`

	// limit caps the number of tenants returned. Default: 20, max: 100.
	Limit int32 `json:"limit,omitempty"`
}

func (r *GetTenantLeaderboardRequest) GetProjectId() string {
	if r != nil {
		return r.ProjectId
	}
	return ""
}

func (r *GetTenantLeaderboardRequest) GetTimeRange() *types.TimeRange {
	if r != nil {
		return r.TimeRange
	}
	return nil
}

func (r *GetTenantLeaderboardRequest) GetLimit() int32 {
	if r != nil {
		return r.Limit
	}
	return 0
}

// GetTenantLeaderboardResponse is the response for the tenant cost leaderboard.
type GetTenantLeaderboardResponse struct {
	// tenants is the ranked list of tenants, ordered by total cost descending.
	Tenants []*TenantUsage `json:"tenants,omitempty"`
}

func (r *GetTenantLeaderboardResponse) GetTenants() []*TenantUsage {
	if r != nil {
		return r.Tenants
	}
	return nil
}

// TenantUsage holds aggregated LLM usage metrics for a single tenant.
// Tenant identity comes from the X-Candela-Tenant-Id header or W3C Baggage
// (candela.tenant_id), set by the calling application.
type TenantUsage struct {
	// tenant_id is the opaque tenant identifier set by the application
	// (e.g. "acme-corp", "trial-NCT01750580", "customer-42").
	TenantId string `json:"tenant_id,omitempty"`

	// call_count is total LLM API calls attributed to this tenant.
	CallCount int64 `json:"call_count,omitempty"`

	// total_tokens is sum of input + output tokens for all calls.
	TotalTokens int64 `json:"total_tokens,omitempty"`

	// cost_usd is total cost in USD for all LLM calls.
	CostUsd float64 `json:"cost_usd,omitempty"`

	// avg_latency_ms is mean end-to-end LLM call latency in milliseconds.
	AvgLatencyMs float64 `json:"avg_latency_ms,omitempty"`

	// top_model is the most-used model (by cost) for this tenant.
	TopModel string `json:"top_model,omitempty"`
}

func (u *TenantUsage) GetTenantId() string {
	if u != nil {
		return u.TenantId
	}
	return ""
}

func (u *TenantUsage) GetCallCount() int64 {
	if u != nil {
		return u.CallCount
	}
	return 0
}

func (u *TenantUsage) GetTotalTokens() int64 {
	if u != nil {
		return u.TotalTokens
	}
	return 0
}

func (u *TenantUsage) GetCostUsd() float64 {
	if u != nil {
		return u.CostUsd
	}
	return 0
}

func (u *TenantUsage) GetAvgLatencyMs() float64 {
	if u != nil {
		return u.AvgLatencyMs
	}
	return 0
}

func (u *TenantUsage) GetTopModel() string {
	if u != nil {
		return u.TopModel
	}
	return ""
}
