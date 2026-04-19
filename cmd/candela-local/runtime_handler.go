package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	connect "connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/candelahq/candela/gen/go/candela/v1"
	"github.com/candelahq/candela/pkg/runtime"
)

// pullStatus tracks an in-flight model pull.
type pullStatus struct {
	mu        sync.Mutex
	cancel    context.CancelFunc // cancels the pull context
	Model     string             `json:"model"`
	Status    string             `json:"status"` // "pulling", "complete", "failed", "cancelled"
	Percent   float64            `json:"percent"`
	Error     string             `json:"error,omitempty"`
	StartedAt string             `json:"startedAt"`
}

// snapshot returns a copy safe for reading without holding the lock.
func (ps *pullStatus) snapshot() pullStatus {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return pullStatus{
		Model:     ps.Model,
		Status:    ps.Status,
		Percent:   ps.Percent,
		Error:     ps.Error,
		StartedAt: ps.StartedAt,
	}
}

// runtimeHandler implements the ConnectRPC RuntimeServiceHandler.
type runtimeHandler struct {
	mgr         *runtime.Manager
	state       *StateDB
	appCtx      context.Context // application-level context for background goroutines
	activePulls sync.Map        // model -> *pullStatus
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

	// Check if already pulling.
	if _, loaded := h.activePulls.Load(model); loaded {
		return connect.NewResponse(&v1.PullModelResponse{Status: "already_pulling"}), nil
	}

	// Register the active pull.
	pullCtx, pullCancel := context.WithCancel(h.appCtx)
	ps := &pullStatus{
		cancel:    pullCancel,
		Model:     model,
		Status:    "pulling",
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	}
	h.activePulls.Store(model, ps)

	// Use a progress channel so we can track download percent.
	progress := make(chan runtime.PullProgress, 16)
	go func() {
		// Drain progress updates.
		go func() {
			for p := range progress {
				ps.mu.Lock()
				ps.Percent = p.Percent
				ps.Status = "pulling"
				ps.mu.Unlock()
			}
		}()

		err := h.mgr.Runtime().PullModel(pullCtx, model, progress)
		close(progress)

		if err != nil {
			ps.mu.Lock()
			if pullCtx.Err() != nil {
				slog.Info("model pull cancelled", "model", model)
				ps.Status = "cancelled"
			} else {
				slog.Error("model pull failed", "model", model, "error", err)
				ps.Status = "failed"
				ps.Error = err.Error()
			}
			ps.mu.Unlock()
			// Keep status visible for 30s then remove — use
			// CompareAndDelete so a re-pull's entry isn't clobbered.
			time.AfterFunc(30*time.Second, func() { h.activePulls.CompareAndDelete(model, ps) })
			return
		}

		slog.Info("model pull complete", "model", model)
		ps.mu.Lock()
		ps.Status = "complete"
		ps.Percent = 100
		ps.mu.Unlock()

		// Record in state DB if available.
		if h.state != nil {
			if err := h.state.RecordPull(model, h.mgr.Runtime().Name(), 0); err != nil {
				slog.Warn("failed to record pull", "model", model, "error", err)
			}
		}

		// Keep completed status visible for 10s then remove.
		time.AfterFunc(10*time.Second, func() { h.activePulls.CompareAndDelete(model, ps) })
	}()

	return connect.NewResponse(&v1.PullModelResponse{Status: "pulling"}), nil
}

// ActivePulls returns a snapshot of all in-flight and recently completed pulls.
func (h *runtimeHandler) ActivePulls() []pullStatus {
	var pulls []pullStatus
	h.activePulls.Range(func(_, value any) bool {
		if ps, ok := value.(*pullStatus); ok {
			pulls = append(pulls, ps.snapshot())
		}
		return true
	})
	return pulls
}

// ServeActivePulls is an HTTP handler that returns active pulls as JSON.
func (h *runtimeHandler) ServeActivePulls(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	pulls := h.ActivePulls()
	if pulls == nil {
		pulls = []pullStatus{}
	}
	_ = json.NewEncoder(w).Encode(pulls)
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

func (h *runtimeHandler) DeleteModel(
	ctx context.Context,
	req *connect.Request[v1.DeleteModelRequest],
) (*connect.Response[v1.DeleteModelResponse], error) {
	if h.mgr == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errNoRuntime)
	}
	model := req.Msg.GetModel()
	if model == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errModelRequired)
	}
	if err := h.mgr.Runtime().DeleteModel(ctx, model); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	slog.Info("model deleted via RPC", "model", model)
	return connect.NewResponse(&v1.DeleteModelResponse{}), nil
}

func (h *runtimeHandler) CancelPull(
	_ context.Context,
	req *connect.Request[v1.CancelPullRequest],
) (*connect.Response[v1.CancelPullResponse], error) {
	model := req.Msg.GetModel()
	if model == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errModelRequired)
	}
	val, ok := h.activePulls.Load(model)
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("no active pull for %q", model))
	}
	ps := val.(*pullStatus)
	ps.mu.Lock()
	if ps.cancel != nil {
		ps.cancel()
	}
	ps.mu.Unlock()
	slog.Info("pull cancelled via RPC", "model", model)
	return connect.NewResponse(&v1.CancelPullResponse{}), nil
}

func (h *runtimeHandler) ListCatalog(
	_ context.Context,
	_ *connect.Request[v1.ListCatalogRequest],
) (*connect.Response[v1.ListCatalogResponse], error) {
	if h.state == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errNoStateDB)
	}
	entries := h.state.ListCatalog()
	resp := &v1.ListCatalogResponse{}
	for _, e := range entries {
		resp.Models = append(resp.Models, &v1.CatalogModel{
			Id:          e.ID,
			Name:        e.Name,
			Description: e.Description,
			SizeHint:    e.SizeHint,
			Pinned:      e.Pinned,
		})
	}
	return connect.NewResponse(resp), nil
}

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
