package connecthandlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	connect "connectrpc.com/connect"
	typespb "github.com/candelahq/candela/gen/go/candela/types"
	v1 "github.com/candelahq/candela/gen/go/candela/v1"
	"github.com/candelahq/candela/gen/go/candela/v1/candelav1connect"
	"github.com/candelahq/candela/pkg/auth"
	"github.com/candelahq/candela/pkg/storage"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// UserHandler implements the UserService ConnectRPC handler,
// backed by a storage.UserStore (Firestore in production).
type UserHandler struct {
	candelav1connect.UnimplementedUserServiceHandler
	store storage.UserStore
}

// NewUserHandler creates a new UserHandler.
func NewUserHandler(store storage.UserStore) *UserHandler {
	return &UserHandler{store: store}
}

// logAudit writes an audit entry and logs a warning if it fails.
func (h *UserHandler) logAudit(ctx context.Context, entry *storage.AuditRecord) {
	if err := h.store.LogAction(ctx, entry); err != nil {
		slog.WarnContext(ctx, "failed to write audit log",
			"error", err,
			"user_id", entry.UserID,
			"action", entry.Action)
	}
}

// ──────────────────────────────────────────
// Admin — User CRUD
// ──────────────────────────────────────────

func (h *UserHandler) CreateUser(
	ctx context.Context,
	req *connect.Request[v1.CreateUserRequest],
) (*connect.Response[v1.CreateUserResponse], error) {
	if req.Msg.Email == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("email is required"))
	}

	// Check for duplicate email.
	normalizedEmail := strings.ToLower(req.Msg.Email)
	if _, err := h.store.GetUserByEmail(ctx, normalizedEmail); err == nil {
		return nil, connect.NewError(connect.CodeAlreadyExists,
			fmt.Errorf("user with email %q already exists", normalizedEmail))
	} else if !errors.Is(err, storage.ErrNotFound) {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to check existing user: %w", err))
	}

	user := &storage.UserRecord{
		Email:       normalizedEmail,
		DisplayName: req.Msg.DisplayName,
		Role:        roleToString(req.Msg.Role),
	}
	if err := h.store.CreateUser(ctx, user); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	resp := &v1.CreateUserResponse{
		User: userToProto(user),
	}

	// Optionally set an initial budget.
	if req.Msg.MonthlyBudgetUsd > 0 {
		budget := &storage.BudgetRecord{
			UserID:     user.ID,
			LimitUSD:   req.Msg.MonthlyBudgetUsd,
			PeriodType: "monthly",
		}
		if err := h.store.SetBudget(ctx, budget); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		resp.Budget = budgetToProto(budget)
	}

	h.logAudit(ctx, &storage.AuditRecord{
		UserID:     user.ID,
		ActorEmail: auth.EmailFromContext(ctx),
		Action:     "create_user",
		Details:    fmt.Sprintf(`{"email":%q}`, user.Email),
	})

	return connect.NewResponse(resp), nil
}

func (h *UserHandler) ListUsers(
	ctx context.Context,
	req *connect.Request[v1.ListUsersRequest],
) (*connect.Response[v1.ListUsersResponse], error) {
	limit, offset := 50, 0
	if req.Msg.Pagination != nil {
		if req.Msg.Pagination.PageSize > 0 {
			limit = int(req.Msg.Pagination.PageSize)
		}
		if req.Msg.Pagination.PageToken != "" {
			// Page token encodes the offset as a decimal string.
			if parsed, err := strconv.Atoi(req.Msg.Pagination.PageToken); err == nil && parsed > 0 {
				offset = parsed
			}
		}
	}

	statusFilter := ""
	if req.Msg.StatusFilter != typespb.UserStatus_USER_STATUS_UNSPECIFIED {
		statusFilter = statusToString(req.Msg.StatusFilter)
	}

	users, total, err := h.store.ListUsers(ctx, statusFilter, limit, offset)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	pbUsers := make([]*typespb.User, len(users))
	for i, u := range users {
		pbUsers[i] = userToProto(u)
	}

	// Build next page token if there are more results.
	var nextPageToken string
	if offset+limit < total {
		nextPageToken = strconv.Itoa(offset + limit)
	}

	return connect.NewResponse(&v1.ListUsersResponse{
		Users: pbUsers,
		Pagination: &typespb.PaginationResponse{
			TotalCount:    int32(total),
			NextPageToken: nextPageToken,
		},
	}), nil
}

func (h *UserHandler) GetUser(
	ctx context.Context,
	req *connect.Request[v1.GetUserRequest],
) (*connect.Response[v1.GetUserResponse], error) {
	user, err := h.store.GetUser(ctx, req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	budget, err := h.store.GetBudget(ctx, user.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get budget: %w", err))
	}
	grants, err := h.store.ListGrants(ctx, user.ID, true)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list grants: %w", err))
	}

	pbGrants := make([]*typespb.BudgetGrant, len(grants))
	for i, g := range grants {
		pbGrants[i] = grantToProto(g)
	}

	return connect.NewResponse(&v1.GetUserResponse{
		User:         userToProto(user),
		Budget:       budgetToProto(budget),
		ActiveGrants: pbGrants,
	}), nil
}

func (h *UserHandler) UpdateUser(
	ctx context.Context,
	req *connect.Request[v1.UpdateUserRequest],
) (*connect.Response[v1.UpdateUserResponse], error) {
	user, err := h.store.GetUser(ctx, req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	// Apply updates based on field mask (or all fields if no mask).
	paths := req.Msg.UpdateMask.GetPaths()
	if len(paths) == 0 {
		// No mask — update all mutable fields.
		paths = []string{"display_name", "role"}
	}
	for _, p := range paths {
		switch p {
		case "display_name":
			user.DisplayName = req.Msg.DisplayName
		case "role":
			user.Role = roleToString(req.Msg.Role)
		}
	}

	if err := h.store.UpdateUser(ctx, user); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&v1.UpdateUserResponse{
		User: userToProto(user),
	}), nil
}

func (h *UserHandler) DeactivateUser(
	ctx context.Context,
	req *connect.Request[v1.DeactivateUserRequest],
) (*connect.Response[v1.DeactivateUserResponse], error) {
	user, err := h.store.GetUser(ctx, req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	user.Status = storage.StatusInactive
	if err := h.store.UpdateUser(ctx, user); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	h.logAudit(ctx, &storage.AuditRecord{
		UserID:     user.ID,
		ActorEmail: auth.EmailFromContext(ctx),
		Action:     "deactivate_user",
	})

	return connect.NewResponse(&v1.DeactivateUserResponse{}), nil
}

func (h *UserHandler) ReactivateUser(
	ctx context.Context,
	req *connect.Request[v1.ReactivateUserRequest],
) (*connect.Response[v1.ReactivateUserResponse], error) {
	user, err := h.store.GetUser(ctx, req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	user.Status = storage.StatusActive
	if err := h.store.UpdateUser(ctx, user); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	h.logAudit(ctx, &storage.AuditRecord{
		UserID:     user.ID,
		ActorEmail: auth.EmailFromContext(ctx),
		Action:     "reactivate_user",
	})

	return connect.NewResponse(&v1.ReactivateUserResponse{
		User: userToProto(user),
	}), nil
}

// ──────────────────────────────────────────
// Admin — Budget management
// ──────────────────────────────────────────

func (h *UserHandler) SetBudget(
	ctx context.Context,
	req *connect.Request[v1.SetBudgetRequest],
) (*connect.Response[v1.SetBudgetResponse], error) {
	budget := &storage.BudgetRecord{
		UserID:     req.Msg.UserId,
		LimitUSD:   req.Msg.LimitUsd,
		PeriodType: periodToString(req.Msg.PeriodType),
	}
	if err := h.store.SetBudget(ctx, budget); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	h.logAudit(ctx, &storage.AuditRecord{
		UserID:     req.Msg.UserId,
		ActorEmail: auth.EmailFromContext(ctx),
		Action:     "set_budget",
		Details:    fmt.Sprintf(`{"limit_usd":%.2f,"period":%q}`, req.Msg.LimitUsd, budget.PeriodType),
	})

	return connect.NewResponse(&v1.SetBudgetResponse{
		Budget: budgetToProto(budget),
	}), nil
}

func (h *UserHandler) GetBudget(
	ctx context.Context,
	req *connect.Request[v1.GetBudgetRequest],
) (*connect.Response[v1.GetBudgetResponse], error) {
	budget, err := h.store.GetBudget(ctx, req.Msg.UserId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.GetBudgetResponse{
		Budget: budgetToProto(budget),
	}), nil
}

func (h *UserHandler) ResetSpend(
	ctx context.Context,
	req *connect.Request[v1.ResetSpendRequest],
) (*connect.Response[v1.ResetSpendResponse], error) {
	if err := h.store.ResetSpend(ctx, req.Msg.UserId); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	h.logAudit(ctx, &storage.AuditRecord{
		UserID:     req.Msg.UserId,
		ActorEmail: auth.EmailFromContext(ctx),
		Action:     "reset_spend",
	})

	budget, err := h.store.GetBudget(ctx, req.Msg.UserId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get budget after reset: %w", err))
	}
	return connect.NewResponse(&v1.ResetSpendResponse{
		Budget: budgetToProto(budget),
	}), nil
}

// ──────────────────────────────────────────
// Admin — Grants
// ──────────────────────────────────────────

func (h *UserHandler) CreateGrant(
	ctx context.Context,
	req *connect.Request[v1.CreateGrantRequest],
) (*connect.Response[v1.CreateGrantResponse], error) {
	grant := &storage.GrantRecord{
		UserID:    req.Msg.UserId,
		AmountUSD: req.Msg.AmountUsd,
		Reason:    req.Msg.Reason,
		GrantedBy: auth.EmailFromContext(ctx),
		StartsAt:  req.Msg.StartsAt.AsTime(),
		ExpiresAt: req.Msg.ExpiresAt.AsTime(),
	}
	if err := h.store.CreateGrant(ctx, grant); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	h.logAudit(ctx, &storage.AuditRecord{
		UserID:     req.Msg.UserId,
		ActorEmail: auth.EmailFromContext(ctx),
		Action:     "create_grant",
		Details:    fmt.Sprintf(`{"amount_usd":%.2f,"reason":%q}`, grant.AmountUSD, grant.Reason),
	})

	return connect.NewResponse(&v1.CreateGrantResponse{
		Grant: grantToProto(grant),
	}), nil
}

func (h *UserHandler) ListGrants(
	ctx context.Context,
	req *connect.Request[v1.ListGrantsRequest],
) (*connect.Response[v1.ListGrantsResponse], error) {
	grants, err := h.store.ListGrants(ctx, req.Msg.UserId, req.Msg.ActiveOnly)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	pbGrants := make([]*typespb.BudgetGrant, len(grants))
	for i, g := range grants {
		pbGrants[i] = grantToProto(g)
	}

	return connect.NewResponse(&v1.ListGrantsResponse{
		Grants: pbGrants,
	}), nil
}

func (h *UserHandler) RevokeGrant(
	ctx context.Context,
	req *connect.Request[v1.RevokeGrantRequest],
) (*connect.Response[v1.RevokeGrantResponse], error) {
	if err := h.store.RevokeGrant(ctx, req.Msg.UserId, req.Msg.GrantId); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	h.logAudit(ctx, &storage.AuditRecord{
		UserID:     req.Msg.UserId,
		ActorEmail: auth.EmailFromContext(ctx),
		Action:     "revoke_grant",
		Details:    fmt.Sprintf(`{"grant_id":%q}`, req.Msg.GrantId),
	})

	return connect.NewResponse(&v1.RevokeGrantResponse{}), nil
}

// ──────────────────────────────────────────
// Admin — Audit
// ──────────────────────────────────────────

func (h *UserHandler) ListAuditLog(
	ctx context.Context,
	req *connect.Request[v1.ListAuditLogRequest],
) (*connect.Response[v1.ListAuditLogResponse], error) {
	entries, err := h.store.ListAuditLog(ctx, req.Msg.UserId, int(req.Msg.Limit))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	pbEntries := make([]*typespb.AuditEntry, len(entries))
	for i, e := range entries {
		pbEntries[i] = auditToProto(e)
	}

	return connect.NewResponse(&v1.ListAuditLogResponse{
		Entries: pbEntries,
	}), nil
}

// ──────────────────────────────────────────
// Self-service — Current user
// ──────────────────────────────────────────

func (h *UserHandler) GetCurrentUser(
	ctx context.Context,
	req *connect.Request[v1.GetCurrentUserRequest],
) (*connect.Response[v1.GetCurrentUserResponse], error) {
	authUser := auth.FromContext(ctx)
	if authUser == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("not authenticated"))
	}

	// Look up or auto-provision the user.
	user, err := h.store.GetUserByEmail(ctx, authUser.Email)
	if err != nil {
		// Only auto-provision if the error is specifically "not found".
		// Other errors (e.g., transient DB issues) should propagate.
		if !errors.Is(err, storage.ErrNotFound) {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to look up user: %w", err))
		}

		// Auto-provision on first login.
		user = &storage.UserRecord{
			Email:  authUser.Email,
			Role:   "developer",
			Status: storage.StatusActive,
		}
		if createErr := h.store.CreateUser(ctx, user); createErr != nil {
			return nil, connect.NewError(connect.CodeInternal, createErr)
		}
	}

	// Update last seen.
	if err := h.store.TouchLastSeen(ctx, user.ID); err != nil {
		slog.WarnContext(ctx, "failed to update last_seen", "error", err, "user_id", user.ID)
	}

	budget, err := h.store.GetBudget(ctx, user.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get budget: %w", err))
	}
	grants, err := h.store.ListGrants(ctx, user.ID, true)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list grants: %w", err))
	}
	check, err := h.store.CheckBudget(ctx, user.ID, 0)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to check budget: %w", err))
	}

	pbGrants := make([]*typespb.BudgetGrant, len(grants))
	for i, g := range grants {
		pbGrants[i] = grantToProto(g)
	}

	var totalRemaining float64
	if check != nil {
		totalRemaining = check.RemainingUSD
	}

	return connect.NewResponse(&v1.GetCurrentUserResponse{
		User:              userToProto(user),
		Budget:            budgetToProto(budget),
		ActiveGrants:      pbGrants,
		TotalRemainingUsd: totalRemaining,
	}), nil
}

func (h *UserHandler) GetMyBudget(
	ctx context.Context,
	req *connect.Request[v1.GetMyBudgetRequest],
) (*connect.Response[v1.GetMyBudgetResponse], error) {
	authUser := auth.FromContext(ctx)
	if authUser == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("not authenticated"))
	}

	user, err := h.store.GetUserByEmail(ctx, authUser.Email)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	budget, err := h.store.GetBudget(ctx, user.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get budget: %w", err))
	}
	grants, err := h.store.ListGrants(ctx, user.ID, true)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list grants: %w", err))
	}
	check, err := h.store.CheckBudget(ctx, user.ID, 0)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to check budget: %w", err))
	}

	pbGrants := make([]*typespb.BudgetGrant, len(grants))
	for i, g := range grants {
		pbGrants[i] = grantToProto(g)
	}

	var totalRemaining float64
	if check != nil {
		totalRemaining = check.RemainingUSD
	}

	return connect.NewResponse(&v1.GetMyBudgetResponse{
		Budget:            budgetToProto(budget),
		ActiveGrants:      pbGrants,
		TotalRemainingUsd: totalRemaining,
	}), nil
}

// ──────────────────────────────────────────
// Proto converters
// ──────────────────────────────────────────

func userToProto(u *storage.UserRecord) *typespb.User {
	if u == nil {
		return nil
	}
	pb := &typespb.User{
		Id:          u.ID,
		Email:       u.Email,
		DisplayName: u.DisplayName,
		Role:        stringToRole(u.Role),
		Status:      stringToStatus(u.Status),
		CreatedAt:   timestamppb.New(u.CreatedAt),
		RateLimit:   int32(u.RateLimit),
	}
	if !u.LastSeenAt.IsZero() {
		pb.LastSeenAt = timestamppb.New(u.LastSeenAt)
	}
	return pb
}

func budgetToProto(b *storage.BudgetRecord) *typespb.UserBudget {
	if b == nil {
		return nil
	}
	return &typespb.UserBudget{
		UserId:      b.UserID,
		LimitUsd:    b.LimitUSD,
		SpentUsd:    b.SpentUSD,
		TokensUsed:  b.TokensUsed,
		PeriodType:  stringToPeriod(b.PeriodType),
		PeriodKey:   b.PeriodKey,
		PeriodStart: timestamppb.New(b.PeriodStart),
		PeriodEnd:   timestamppb.New(b.PeriodEnd),
	}
}

func grantToProto(g *storage.GrantRecord) *typespb.BudgetGrant {
	if g == nil {
		return nil
	}
	return &typespb.BudgetGrant{
		Id:        g.ID,
		UserId:    g.UserID,
		AmountUsd: g.AmountUSD,
		SpentUsd:  g.SpentUSD,
		Reason:    g.Reason,
		GrantedBy: g.GrantedBy,
		StartsAt:  timestamppb.New(g.StartsAt),
		ExpiresAt: timestamppb.New(g.ExpiresAt),
		CreatedAt: timestamppb.New(g.CreatedAt),
	}
}

func auditToProto(a *storage.AuditRecord) *typespb.AuditEntry {
	if a == nil {
		return nil
	}
	return &typespb.AuditEntry{
		Id:         a.ID,
		UserId:     a.UserID,
		ActorEmail: a.ActorEmail,
		Action:     a.Action,
		Details:    a.Details,
		Timestamp:  timestamppb.New(a.Timestamp),
	}
}

// ── Enum converters ──

func roleToString(r typespb.UserRole) string {
	switch r {
	case typespb.UserRole_USER_ROLE_ADMIN:
		return "admin"
	default:
		return "developer"
	}
}

func stringToRole(s string) typespb.UserRole {
	switch s {
	case "admin":
		return typespb.UserRole_USER_ROLE_ADMIN
	default:
		return typespb.UserRole_USER_ROLE_DEVELOPER
	}
}

func statusToString(s typespb.UserStatus) string {
	switch s {
	case typespb.UserStatus_USER_STATUS_PROVISIONED:
		return storage.StatusProvisioned
	case typespb.UserStatus_USER_STATUS_ACTIVE:
		return storage.StatusActive
	case typespb.UserStatus_USER_STATUS_INACTIVE:
		return storage.StatusInactive
	default:
		return ""
	}
}

func stringToStatus(s string) typespb.UserStatus {
	switch s {
	case storage.StatusProvisioned:
		return typespb.UserStatus_USER_STATUS_PROVISIONED
	case storage.StatusActive:
		return typespb.UserStatus_USER_STATUS_ACTIVE
	case storage.StatusInactive:
		return typespb.UserStatus_USER_STATUS_INACTIVE
	default:
		return typespb.UserStatus_USER_STATUS_UNSPECIFIED
	}
}

func periodToString(p typespb.BudgetPeriod) string {
	switch p {
	case typespb.BudgetPeriod_BUDGET_PERIOD_WEEKLY:
		return "weekly"
	case typespb.BudgetPeriod_BUDGET_PERIOD_QUARTERLY:
		return "quarterly"
	default:
		return "monthly"
	}
}

func stringToPeriod(s string) typespb.BudgetPeriod {
	switch s {
	case "weekly":
		return typespb.BudgetPeriod_BUDGET_PERIOD_WEEKLY
	case "quarterly":
		return typespb.BudgetPeriod_BUDGET_PERIOD_QUARTERLY
	case "monthly":
		return typespb.BudgetPeriod_BUDGET_PERIOD_MONTHLY
	default:
		return typespb.BudgetPeriod_BUDGET_PERIOD_UNSPECIFIED
	}
}
