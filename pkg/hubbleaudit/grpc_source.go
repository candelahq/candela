package hubbleaudit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/rand/v2"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding"
)

// bytesCodec is a gRPC codec for raw byte transfer without protobuf.
// This is registered locally so hubbleaudit can be used independently
// of the tetragonaudit package.
type bytesCodec struct{}

func (bytesCodec) Marshal(v any) ([]byte, error) {
	switch b := v.(type) {
	case []byte:
		return b, nil
	case json.RawMessage:
		return []byte(b), nil
	default:
		return nil, fmt.Errorf("bytesCodec: unsupported type %T", v)
	}
}

func (bytesCodec) Unmarshal(data []byte, v any) error {
	switch p := v.(type) {
	case *[]byte:
		// Copy data — gRPC transport buffers may be reused.
		*p = append((*p)[:0], data...)
		return nil
	case *json.RawMessage:
		*p = append((*p)[:0], data...)
		return nil
	default:
		return fmt.Errorf("bytesCodec: unsupported type %T", v)
	}
}

func (bytesCodec) Name() string { return "hubble-bytes" }

func init() {
	encoding.RegisterCodec(bytesCodec{})
}

// GRPCFlowSourceConfig configures the Hubble gRPC flow source.
type GRPCFlowSourceConfig struct {
	// Addr is the Hubble relay gRPC endpoint
	// (e.g. "hubble-relay.kube-system.svc:4245").
	Addr string

	// DialOpts are additional gRPC dial options. If empty,
	// insecure credentials are used.
	DialOpts []grpc.DialOption

	// Filter is the default flow filter applied on Observe().
	Filter FlowFilter
}

// GRPCFlowSource implements FlowSource by connecting to the Hubble relay
// gRPC endpoint and streaming network flows. It uses the same bytesCodec
// registered by the tetragonaudit package for proto-free transport.
type GRPCFlowSource struct {
	cfg  GRPCFlowSourceConfig
	conn *grpc.ClientConn

	mu        sync.Mutex
	connected bool
	lastFlow  time.Time
}

// NewGRPCFlowSource creates a new Hubble gRPC flow source.
func NewGRPCFlowSource(cfg GRPCFlowSourceConfig) (*GRPCFlowSource, error) {
	if cfg.Addr == "" {
		return nil, fmt.Errorf("hubbleaudit: gRPC address is required")
	}
	opts := cfg.DialOpts
	if len(opts) == 0 {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	conn, err := grpc.NewClient(cfg.Addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("hubbleaudit: dial gRPC %s: %w", cfg.Addr, err)
	}
	return &GRPCFlowSource{cfg: cfg, conn: conn}, nil
}

// Observe starts streaming Hubble flows matching the given filter.
// The returned channel is closed when the context is cancelled or the
// stream encounters a fatal error.
func (s *GRPCFlowSource) Observe(ctx context.Context, filter FlowFilter) (<-chan Flow, error) {
	stream, err := s.newStream(ctx, filter)
	if err != nil {
		return nil, err
	}

	ch := make(chan Flow, 64)
	go s.readLoop(ctx, stream, ch)
	return ch, nil
}

// flowRequest is the JSON representation of a Hubble GetFlowsRequest.
// Fields mirror the Hubble proto without requiring a proto dependency.
type flowRequest struct {
	Whitelist []flowFilterEntry `json:"whitelist,omitempty"`
	Since     *time.Time        `json:"since,omitempty"`
	Follow    bool              `json:"follow,omitempty"`
}

type flowFilterEntry struct {
	SourcePod []string `json:"source_pod,omitempty"`
	DestPod   []string `json:"destination_pod,omitempty"`
	Verdict   []string `json:"verdict,omitempty"`
	Type      []string `json:"type,omitempty"`
}

// buildRequest translates a FlowFilter into the JSON request body.
// Pod/namespace filters use separate whitelist entries for source and
// destination to achieve OR semantics ("source OR destination matches").
func buildRequest(filter FlowFilter) ([]byte, error) {
	req := flowRequest{
		Since:  filter.Since,
		Follow: true,
	}

	newEntry := func() flowFilterEntry {
		return flowFilterEntry{
			Verdict: filter.Verdicts,
		}
	}

	// Build pod filter from namespace/podname if provided.
	// Use separate entries for source and destination to achieve OR logic;
	// combining both in one entry would create AND semantics.
	if filter.PodName != "" {
		podFilter := filter.PodName
		if filter.Namespace != "" {
			podFilter = filter.Namespace + "/" + filter.PodName
		}
		e1 := newEntry()
		e1.SourcePod = []string{podFilter}
		req.Whitelist = append(req.Whitelist, e1)

		e2 := newEntry()
		e2.DestPod = []string{podFilter}
		req.Whitelist = append(req.Whitelist, e2)
	} else if filter.Namespace != "" {
		// Namespace-only filter uses wildcard pod matching.
		nsFilter := filter.Namespace + "/"
		e1 := newEntry()
		e1.SourcePod = []string{nsFilter}
		req.Whitelist = append(req.Whitelist, e1)

		e2 := newEntry()
		e2.DestPod = []string{nsFilter}
		req.Whitelist = append(req.Whitelist, e2)
	} else if len(filter.Verdicts) > 0 {
		req.Whitelist = append(req.Whitelist, newEntry())
	}

	return json.Marshal(req)
}

// newStream creates a server-streaming RPC on the Hubble Observer/GetFlows
// endpoint using the locally registered bytesCodec.
func (s *GRPCFlowSource) newStream(ctx context.Context, filter FlowFilter) (grpc.ClientStream, error) {
	if s.conn == nil {
		return nil, fmt.Errorf("hubbleaudit: gRPC connection is nil")
	}
	desc := &grpc.StreamDesc{ServerStreams: true}
	codec := encoding.GetCodec("hubble-bytes")
	stream, err := s.conn.NewStream(ctx, desc, "/observer.Observer/GetFlows",
		grpc.ForceCodec(codec))
	if err != nil {
		return nil, fmt.Errorf("hubbleaudit: failed to create gRPC stream: %w", err)
	}
	// Marshal and send the filter as the GetFlows request.
	reqBody, err := buildRequest(filter)
	if err != nil {
		return nil, fmt.Errorf("hubbleaudit: failed to marshal GetFlows request: %w", err)
	}
	if err := stream.SendMsg(reqBody); err != nil {
		return nil, fmt.Errorf("hubbleaudit: failed to send GetFlows request: %w", err)
	}
	if err := stream.CloseSend(); err != nil {
		return nil, fmt.Errorf("hubbleaudit: failed to close send: %w", err)
	}
	return stream, nil
}

// readLoop reads flows from the gRPC stream and sends them to the channel.
func (s *GRPCFlowSource) readLoop(ctx context.Context, stream grpc.ClientStream, ch chan<- Flow) {
	defer close(ch)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		var raw json.RawMessage
		if err := stream.RecvMsg(&raw); err != nil {
			if err != io.EOF {
				slog.Warn("hubbleaudit: gRPC recv error", "error", err)
			}
			return
		}

		var flow Flow
		if err := json.Unmarshal(raw, &flow); err != nil {
			slog.Debug("hubbleaudit: failed to unmarshal flow", "error", err)
			continue
		}

		s.mu.Lock()
		s.lastFlow = time.Now()
		s.mu.Unlock()

		select {
		case ch <- flow:
		case <-ctx.Done():
			return
		}
	}
}

// ── Retry / Health ──

// RetryConfig controls the reconnect backoff behavior.
type RetryConfig struct {
	// InitialDelay is the first backoff duration (default: 1s).
	InitialDelay time.Duration
	// MaxDelay is the backoff ceiling (default: 30s).
	MaxDelay time.Duration
	// Multiplier is the backoff factor (default: 2.0).
	Multiplier float64
}

func (rc RetryConfig) withDefaults() RetryConfig {
	if rc.InitialDelay <= 0 {
		rc.InitialDelay = time.Second
	}
	if rc.MaxDelay <= 0 {
		rc.MaxDelay = 30 * time.Second
	}
	if rc.Multiplier <= 0 {
		rc.Multiplier = 2.0
	}
	return rc
}

// FlowHandler is called for each flow received from the Hubble stream.
// Returning an error logs a warning but does not stop the stream.
type FlowHandler func(ctx context.Context, flow Flow) error

// StreamWithRetry streams Hubble flows with automatic reconnection using
// exponential backoff. handler is called for each flow. The loop exits
// when ctx is cancelled.
func (s *GRPCFlowSource) StreamWithRetry(ctx context.Context, handler FlowHandler, rc RetryConfig) error {
	rc = rc.withDefaults()
	attempt := 0

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		stream, err := s.newStream(ctx, s.cfg.Filter)
		if err != nil {
			s.SetConnected(false)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			delay := backoff(attempt, rc)
			slog.Warn("hubbleaudit: gRPC stream creation failed, retrying",
				"error", err, "attempt", attempt+1, "backoff", delay)
			attempt++
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
				continue
			}
		}

		slog.Info("hubbleaudit: gRPC stream connected", "addr", s.cfg.Addr, "attempt", attempt+1)
		s.SetConnected(true)
		attempt = 0

		err = s.processStream(ctx, stream, handler)
		s.SetConnected(false)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err == nil {
			// Clean EOF — reconnect after initial delay.
			slog.Info("hubbleaudit: gRPC stream ended, reconnecting", "addr", s.cfg.Addr)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(rc.InitialDelay):
				continue
			}
		}

		delay := backoff(attempt, rc)
		slog.Warn("hubbleaudit: gRPC stream error, reconnecting",
			"error", err, "attempt", attempt+1, "backoff", delay)
		attempt++
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
}

// processStream reads flows from a connected stream and calls the handler.
func (s *GRPCFlowSource) processStream(ctx context.Context, stream grpc.ClientStream, handler FlowHandler) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var raw json.RawMessage
		if err := stream.RecvMsg(&raw); err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("hubbleaudit: gRPC recv: %w", err)
		}

		var flow Flow
		if err := json.Unmarshal(raw, &flow); err != nil {
			slog.Debug("hubbleaudit: failed to unmarshal flow", "error", err)
			continue
		}

		s.mu.Lock()
		s.lastFlow = time.Now()
		s.mu.Unlock()

		if err := handler(ctx, flow); err != nil {
			slog.Warn("hubbleaudit: flow handler error", "error", err)
		}
	}
}

// FlowSourceHealth reports the Hubble flow source health.
type FlowSourceHealth struct {
	Connected  bool      `json:"connected"`
	LastFlowAt time.Time `json:"last_flow_at,omitempty"`
}

// Health returns the current health of the Hubble flow source.
func (s *GRPCFlowSource) Health() FlowSourceHealth {
	s.mu.Lock()
	defer s.mu.Unlock()
	return FlowSourceHealth{
		Connected:  s.connected,
		LastFlowAt: s.lastFlow,
	}
}

// SetConnected updates the connection status.
func (s *GRPCFlowSource) SetConnected(connected bool) {
	s.mu.Lock()
	s.connected = connected
	s.mu.Unlock()
}

// Close closes the gRPC connection.
func (s *GRPCFlowSource) Close() error {
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

// Addr returns the configured gRPC address.
func (s *GRPCFlowSource) Addr() string {
	return s.cfg.Addr
}

// backoff computes the delay for a given attempt with ±25% jitter.
func backoff(attempt int, rc RetryConfig) time.Duration {
	d := float64(rc.InitialDelay) * math.Pow(rc.Multiplier, float64(attempt))
	if d > float64(rc.MaxDelay) {
		d = float64(rc.MaxDelay)
	}
	jitter := d * 0.25 * (2*rand.Float64() - 1) //nolint:gosec
	return time.Duration(d + jitter)
}
