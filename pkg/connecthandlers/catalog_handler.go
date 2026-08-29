package connecthandlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	connect "connectrpc.com/connect"
	types "github.com/candelahq/candela/gen/go/candela/types"
	domain "github.com/candelahq/candela/gen/go/candela/types/domain"
	v1 "github.com/candelahq/candela/gen/go/candela/v1"
	"github.com/candelahq/candela/pkg/catalog"
	"github.com/candelahq/candela/pkg/storage"
)

// CatalogHandler implements the ModelCatalogService ConnectRPC handler.
type CatalogHandler struct {
	store catalog.ModelCatalogStore
	users storage.UserStore // optional, nil in local dev
}

// NewCatalogHandler creates a new CatalogHandler.
func NewCatalogHandler(store catalog.ModelCatalogStore, users storage.UserStore) *CatalogHandler {
	return &CatalogHandler{store: store, users: users}
}

// ListModelCatalog returns all models in the catalog.
// Non-admin callers always receive enabled-only models regardless of include_disabled.
// Models with required_access tags are filtered to only show models the caller can access.
func (h *CatalogHandler) ListModelCatalog(
	ctx context.Context,
	req *connect.Request[v1.ListModelCatalogRequest],
) (*connect.Response[v1.ListModelCatalogResponse], error) {
	includeDisabled := req.Msg.IncludeDisabled

	// Resolve caller scope once — empty string means admin (full access).
	callerScope := scopeUserID(ctx, h.users)

	// Non-admin callers cannot see disabled models.
	if callerScope != "" {
		includeDisabled = false
	}

	entries, err := h.store.List(ctx, includeDisabled)
	if err != nil {
		return nil, internalError("failed to list model catalog", err)
	}

	// For non-admin callers, filter out models they don't have access to.
	// Fail closed: if the caller record can't be resolved, treat them as
	// having no access tags rather than skipping filtering entirely.
	if callerScope != "" && h.users != nil {
		var userTags []string
		u, err := h.users.GetUser(ctx, callerScope)
		switch {
		case err == nil && u != nil:
			userTags = u.AccessTags
		case err != nil && !errors.Is(err, storage.ErrNotFound):
			slog.Warn("failed to look up caller for access filtering, failing closed",
				"user", callerScope, "error", err)
		}
		entries = filterEntriesByAccess(entries, userTags)
	}

	pbModels := make([]*types.ModelCatalogEntry, len(entries))
	for i, e := range entries {
		pbModels[i] = e.ToProto()
	}

	return connect.NewResponse(&v1.ListModelCatalogResponse{
		Models:        pbModels,
		Source:        h.store.Source(),
		AdminEditable: h.store.Writable() && callerScope == "",
	}), nil
}

// filterEntriesByAccess removes entries the user can't access based on
// required_access tags. Models with empty required_access are always visible.
func filterEntriesByAccess(entries []catalog.Entry, userTags []string) []catalog.Entry {
	tagSet := make(map[string]bool, len(userTags))
	for _, t := range userTags {
		tagSet[t] = true
	}
	var filtered []catalog.Entry
	for _, e := range entries {
		if len(e.RequiredAccess) == 0 {
			filtered = append(filtered, e)
			continue
		}
		for _, tag := range e.RequiredAccess {
			if tagSet[tag] {
				filtered = append(filtered, e)
				break
			}
		}
	}
	return filtered
}

// UpdateModelCatalogEntry updates a single model entry.
// Returns Unimplemented when the catalog backend is read-only.
func (h *CatalogHandler) UpdateModelCatalogEntry(
	ctx context.Context,
	req *connect.Request[v1.UpdateModelCatalogEntryRequest],
) (*connect.Response[v1.UpdateModelCatalogEntryResponse], error) {
	if !h.store.Writable() {
		return nil, connect.NewError(connect.CodeUnimplemented,
			fmt.Errorf("catalog backend (%s) is read-only", h.store.Source()))
	}

	// Admin-only guard.
	if uid := scopeUserID(ctx, h.users); uid != "" {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("admin access required"))
	}

	pbEntry := req.Msg.Entry
	if pbEntry == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("entry is required"))
	}

	var entry catalog.Entry
	entry.FromProto(pbEntry)

	// Apply field mask: if the caller specified which fields to update,
	// merge only those fields onto the existing entry to avoid data loss.
	if mask := req.Msg.UpdateMask; mask != nil && len(mask.Paths) > 0 {
		existing, err := h.store.Get(ctx, entry.Provider, entry.ModelID)
		if err != nil {
			if errors.Is(err, catalog.ErrNotFound) {
				return nil, connect.NewError(connect.CodeNotFound,
					fmt.Errorf("model %s/%s not found", entry.Provider, entry.ModelID))
			}
			return nil, internalError("failed to fetch existing entry for field mask", err)
		}
		domain.ApplyFieldMaskModelCatalogEntry(existing, &entry, mask.Paths)
		entry = *existing
	}

	if err := h.store.Update(ctx, entry); err != nil {
		return nil, internalError("failed to update catalog entry", err)
	}

	// Re-fetch the entry to return the server-set fields (e.g. updated_at).
	updated, err := h.store.Get(ctx, entry.Provider, entry.ModelID)
	if err != nil {
		slog.Warn("re-fetch after update failed, returning input entry",
			"provider", entry.Provider, "model_id", entry.ModelID, "error", err)
		return connect.NewResponse(&v1.UpdateModelCatalogEntryResponse{
			Entry: entry.ToProto(),
		}), nil
	}

	return connect.NewResponse(&v1.UpdateModelCatalogEntryResponse{
		Entry: updated.ToProto(),
	}), nil
}

// DeleteModelCatalogEntry removes a model entry from the catalog.
// Returns Unimplemented when the catalog backend is read-only.
func (h *CatalogHandler) DeleteModelCatalogEntry(
	ctx context.Context,
	req *connect.Request[v1.DeleteModelCatalogEntryRequest],
) (*connect.Response[v1.DeleteModelCatalogEntryResponse], error) {
	if !h.store.Writable() {
		return nil, connect.NewError(connect.CodeUnimplemented,
			fmt.Errorf("catalog backend (%s) is read-only", h.store.Source()))
	}

	// Admin-only guard.
	if uid := scopeUserID(ctx, h.users); uid != "" {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("admin access required"))
	}

	provider := req.Msg.Provider
	modelID := req.Msg.ModelId

	if err := h.store.Delete(ctx, provider, modelID); err != nil {
		if errors.Is(err, catalog.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound,
				fmt.Errorf("model %s/%s not found", provider, modelID))
		}
		return nil, internalError("failed to delete catalog entry", err)
	}

	return connect.NewResponse(&v1.DeleteModelCatalogEntryResponse{}), nil
}
