package connecthandlers

import (
	"context"
	"fmt"
	"log/slog"

	connect "connectrpc.com/connect"
	types "github.com/candelahq/candela/gen/go/candela/types"
	v1 "github.com/candelahq/candela/gen/go/candela/v1"
	"github.com/candelahq/candela/pkg/catalog"
	"github.com/candelahq/candela/pkg/storage"
	"google.golang.org/protobuf/types/known/timestamppb"
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
func (h *CatalogHandler) ListModelCatalog(
	ctx context.Context,
	req *connect.Request[v1.ListModelCatalogRequest],
) (*connect.Response[v1.ListModelCatalogResponse], error) {
	includeDisabled := req.Msg.IncludeDisabled

	// Non-admin callers cannot see disabled models.
	if uid := scopeUserID(ctx, h.users); uid != "" {
		includeDisabled = false
	}

	entries, err := h.store.List(ctx, includeDisabled)
	if err != nil {
		return nil, internalError("failed to list model catalog", err)
	}

	pbModels := make([]*types.ModelCatalogEntry, len(entries))
	for i, e := range entries {
		pbModels[i] = entryToProto(&e)
	}

	return connect.NewResponse(&v1.ListModelCatalogResponse{
		Models:        pbModels,
		Source:        h.store.Source(),
		AdminEditable: h.store.Writable(),
	}), nil
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

	entry := protoToEntry(pbEntry)

	// Apply field mask: if the caller specified which fields to update,
	// merge only those fields onto the existing entry to avoid data loss.
	if mask := req.Msg.UpdateMask; mask != nil && len(mask.Paths) > 0 {
		existing, err := h.store.Get(ctx, entry.Provider, entry.ModelID)
		if err != nil {
			if err == catalog.ErrNotFound {
				return nil, connect.NewError(connect.CodeNotFound,
					fmt.Errorf("model %s/%s not found", entry.Provider, entry.ModelID))
			}
			return nil, internalError("failed to fetch existing entry for field mask", err)
		}
		entry = applyFieldMask(*existing, entry, mask.Paths)
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
			Entry: entryToProto(&entry),
		}), nil
	}

	return connect.NewResponse(&v1.UpdateModelCatalogEntryResponse{
		Entry: entryToProto(updated),
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
		if err == catalog.ErrNotFound {
			return nil, connect.NewError(connect.CodeNotFound,
				fmt.Errorf("model %s/%s not found", provider, modelID))
		}
		return nil, internalError("failed to delete catalog entry", err)
	}

	return connect.NewResponse(&v1.DeleteModelCatalogEntryResponse{}), nil
}

// applyFieldMask merges only the specified fields from src into dst.
// Unrecognised paths are silently ignored (the proto validator should
// catch invalid paths before they reach here).
func applyFieldMask(dst, src catalog.Entry, paths []string) catalog.Entry {
	for _, path := range paths {
		switch path {
		case "enabled":
			dst.Enabled = src.Enabled
		case "display_name":
			dst.DisplayName = src.DisplayName
		case "input_per_million":
			dst.InputPerMillion = src.InputPerMillion
		case "output_per_million":
			dst.OutputPerMillion = src.OutputPerMillion
		case "input_per_million_high":
			dst.InputPerMillionHigh = src.InputPerMillionHigh
		case "output_per_million_high":
			dst.OutputPerMillionHigh = src.OutputPerMillionHigh
		case "tier_threshold_tokens":
			dst.TierThresholdTokens = src.TierThresholdTokens
		case "discount_percent":
			dst.DiscountPercent = src.DiscountPercent
		case "category":
			dst.Category = src.Category
		case "context_window":
			dst.ContextWindow = src.ContextWindow
		case "aliases":
			dst.Aliases = src.Aliases
		case "allowed_tenants":
			dst.AllowedTenants = src.AllowedTenants
		}
	}
	return dst
}

// --- Proto converters ---

// entryToProto converts a catalog.Entry to a proto ModelCatalogEntry.
func entryToProto(e *catalog.Entry) *types.ModelCatalogEntry {
	pb := &types.ModelCatalogEntry{
		ModelId:              e.ModelID,
		Provider:             e.Provider,
		DisplayName:          e.DisplayName,
		InputPerMillion:      e.InputPerMillion,
		OutputPerMillion:     e.OutputPerMillion,
		Enabled:              e.Enabled,
		Category:             e.Category,
		ContextWindow:        e.ContextWindow,
		InputPerMillionHigh:  e.InputPerMillionHigh,
		OutputPerMillionHigh: e.OutputPerMillionHigh,
		TierThresholdTokens:  e.TierThresholdTokens,
		Aliases:              e.Aliases,
		AllowedTenants:       e.AllowedTenants,
		DiscountPercent:      e.DiscountPercent,
	}
	if !e.UpdatedAt.IsZero() {
		pb.UpdatedAt = timestamppb.New(e.UpdatedAt)
	}
	return pb
}

// protoToEntry converts a proto ModelCatalogEntry to a catalog.Entry.
func protoToEntry(pb *types.ModelCatalogEntry) catalog.Entry {
	e := catalog.Entry{
		ModelID:              pb.ModelId,
		Provider:             pb.Provider,
		DisplayName:          pb.DisplayName,
		InputPerMillion:      pb.InputPerMillion,
		OutputPerMillion:     pb.OutputPerMillion,
		Enabled:              pb.Enabled,
		Category:             pb.Category,
		ContextWindow:        pb.ContextWindow,
		InputPerMillionHigh:  pb.InputPerMillionHigh,
		OutputPerMillionHigh: pb.OutputPerMillionHigh,
		TierThresholdTokens:  pb.TierThresholdTokens,
		Aliases:              pb.Aliases,
		AllowedTenants:       pb.AllowedTenants,
		DiscountPercent:      pb.DiscountPercent,
	}
	if pb.UpdatedAt != nil {
		e.UpdatedAt = pb.UpdatedAt.AsTime()
	}
	return e
}
