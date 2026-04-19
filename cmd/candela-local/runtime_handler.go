package main

import (
	"context"
	"fmt"
	"log/slog"

	connect "connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/candelahq/candela/gen/go/candela/v1"
	"github.com/candelahq/candela/pkg/runtime"
)

// runtimeHandler implements the ConnectRPC RuntimeServiceHandler.
type runtimeHandler struct {
	mgr    *runtime.Manager
	state  *StateDB
	appCtx context.Context // application-level context for background goroutines
}

// newRuntimeHandler creates a handler backed by the given Manager and state DB.
// Either may be nil if not configured (RPCs will return appropriate errors).
// appCtx should be the application's long-lived context so background tasks
// (e.g. model pulls) are cancelled on graceful shutdown.
func newRuntimeHandler(mgr *runtime.Manager, state *StateDB, appCtx context.Context) *runtimeHandler {
	return &runtimeHandler{mgr: mgr, state: state, appCtx: appCtx}
}

func (h *runtimeHandler) StartRuntime(
	ctx context.Context,
	_ *connect.Request[v1.StartRuntimeRequest],
) (*connect.Response[v1.StartRuntimeResponse], error) {
	if h.mgr == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errNoRuntime)
	}
	// Always start the runtime when explicitly requested via RPC,
	// regardless of the auto_start config flag.
	if err := h.mgr.Runtime().Start(ctx); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	// Re-start the health loop so it picks up the new state.
	if err := h.mgr.Start(ctx); err != nil {
		slog.Warn("failed to restart health loop", "error", err)
	}
	return connect.NewResponse(&v1.StartRuntimeResponse{
		Status: healthToProto(h.mgr.Health(), h.mgr.Runtime().Name()),
	}), nil
}

func (h *runtimeHandler) StopRuntime(
	ctx context.Context,
	_ *connect.Request[v1.StopRuntimeRequest],
) (*connect.Response[v1.StopRuntimeResponse], error) {
	if h.mgr == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errNoRuntime)
	}
	if err := h.mgr.Stop(ctx); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.StopRuntimeResponse{
		Status: healthToProto(h.mgr.Health(), h.mgr.Runtime().Name()),
	}), nil
}

func (h *runtimeHandler) GetHealth(
	_ context.Context,
	_ *connect.Request[v1.GetHealthRequest],
) (*connect.Response[v1.GetHealthResponse], error) {
	if h.mgr == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errNoRuntime)
	}
	health := h.mgr.Health()
	resp := &v1.GetHealthResponse{
		Status: healthToProto(health, h.mgr.Runtime().Name()),
	}
	for _, m := range health.Models {
		resp.Models = append(resp.Models, modelToProto(m))
	}
	return connect.NewResponse(resp), nil
}

func (h *runtimeHandler) LoadModel(
	ctx context.Context,
	req *connect.Request[v1.LoadModelRequest],
) (*connect.Response[v1.LoadModelResponse], error) {
	if h.mgr == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errNoRuntime)
	}
	model := req.Msg.GetModel()
	if model == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errModelRequired)
	}
	if err := h.mgr.LoadModel(ctx, model); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	slog.Info("model loaded via RPC", "model", model)
	return connect.NewResponse(&v1.LoadModelResponse{Status: "loaded"}), nil
}

func (h *runtimeHandler) UnloadModel(
	ctx context.Context,
	req *connect.Request[v1.UnloadModelRequest],
) (*connect.Response[v1.UnloadModelResponse], error) {
	if h.mgr == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errNoRuntime)
	}
	model := req.Msg.GetModel()
	if model == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errModelRequired)
	}
	if err := h.mgr.UnloadModel(ctx, model); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	slog.Info("model unloaded via RPC", "model", model)
	return connect.NewResponse(&v1.UnloadModelResponse{}), nil
}

func (h *runtimeHandler) ListModels(
	ctx context.Context,
	_ *connect.Request[v1.ListModelsRequest],
) (*connect.Response[v1.ListModelsResponse], error) {
	if h.mgr == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errNoRuntime)
	}
	models, err := h.mgr.Runtime().ListModels(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, err)
	}
	resp := &v1.ListModelsResponse{}
	for _, m := range models {
		resp.Models = append(resp.Models, modelToProto(m))
	}
	return connect.NewResponse(resp), nil
}

func (h *runtimeHandler) PullModel(
	_ context.Context,
	req *connect.Request[v1.PullModelRequest],
) (*connect.Response[v1.PullModelResponse], error) {
	if h.mgr == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errNoRuntime)
	}
	model := req.Msg.GetModel()
	if model == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errModelRequired)
	}

	// Run the pull in the background using the app-level context so it
	// survives the HTTP handler but is cancelled on graceful shutdown.
	go func() {
		if err := h.mgr.Runtime().PullModel(h.appCtx, model, nil); err != nil {
			slog.Error("model pull failed", "model", model, "error", err)
			return
		}
		slog.Info("model pull complete", "model", model)
		// Record in state DB if available.
		if h.state != nil {
			if err := h.state.RecordPull(model, h.mgr.Runtime().Name(), 0); err != nil {
				slog.Warn("failed to record pull", "model", model, "error", err)
			}
		}
	}()

	return connect.NewResponse(&v1.PullModelResponse{Status: "pulling"}), nil
}

func (h *runtimeHandler) ListBackends(
	_ context.Context,
	_ *connect.Request[v1.ListBackendsRequest],
) (*connect.Response[v1.ListBackendsResponse], error) {
	discovered := runtime.Discover()
	resp := &v1.ListBackendsResponse{}
	for _, b := range discovered {
		resp.Backends = append(resp.Backends, &v1.BackendInfo{
			Name:        b.Name,
			Installed:   b.Installed,
			BinaryPath:  b.BinaryPath,
			InstallHint: b.InstallHint,
		})
	}
	if h.mgr != nil {
		resp.Active = h.mgr.Runtime().Name()
	}
	return connect.NewResponse(resp), nil
}

func (h *runtimeHandler) ResetState(
	_ context.Context,
	_ *connect.Request[v1.ResetStateRequest],
) (*connect.Response[v1.ResetStateResponse], error) {
	if h.state == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errNoStateDB)
	}
	if err := h.state.Reset(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	slog.Info("state DB reset via RPC")
	return connect.NewResponse(&v1.ResetStateResponse{}), nil
}

// ── Helpers ──

var (
	errNoRuntime     = fmt.Errorf("no runtime backend configured")
	errModelRequired = fmt.Errorf("model is required")
	errNoStateDB     = fmt.Errorf("state database not configured")
)

func healthToProto(h *runtime.Health, backend string) *v1.RuntimeStatus {
	return &v1.RuntimeStatus{
		Status:        string(h.Status),
		Backend:       backend,
		Endpoint:      h.Endpoint,
		UptimeSeconds: h.Uptime,
		Error:         h.Error,
		CheckedAt:     timestamppb.New(h.CheckedAt),
	}
}

func modelToProto(m runtime.Model) *v1.RuntimeModel {
	rm := &v1.RuntimeModel{
		Id:           m.ID,
		SizeBytes:    m.SizeBytes,
		Family:       m.Family,
		Parameters:   m.Parameters,
		Quantization: m.Quantization,
		Loaded:       m.Loaded,
		Available:    true, // If listed, it's available on disk.
	}
	if !m.LastUsed.IsZero() {
		rm.LastUsed = timestamppb.New(m.LastUsed)
	}
	return rm
}
