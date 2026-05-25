package tetragonaudit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/rand/v2"
	"time"

	tetragon "github.com/cilium/tetragon/api/v1/tetragon"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
)

// GRPCSource streams Tetragon events from the Tetragon gRPC export API.
// This is the production event source for in-cluster deployments where
// Tetragon exposes its gRPC endpoint (typically via Unix socket or localhost).
type GRPCSource struct {
	addr string
	conn *grpc.ClientConn
}

// NewGRPCSource creates a new gRPC event source.
// addr is the Tetragon gRPC endpoint (e.g. "localhost:54321" or
// "unix:///var/run/tetragon/tetragon.sock").
func NewGRPCSource(addr string, opts ...grpc.DialOption) (*GRPCSource, error) {
	if addr == "" {
		return nil, fmt.Errorf("tetragonaudit: gRPC address is required")
	}
	if len(opts) == 0 {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	conn, err := grpc.NewClient(addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("tetragonaudit: dial gRPC %s: %w", addr, err)
	}
	return &GRPCSource{addr: addr, conn: conn}, nil
}

// TetragonEventStream is the interface for a Tetragon gRPC event stream.
// This abstracts the Tetragon GetEvents streaming RPC to support testing
// without requiring a live Tetragon instance.
type TetragonEventStream interface {
	// Recv blocks until the next event is available or the stream ends.
	Recv() ([]byte, error)
}

// StreamEvents reads events from a TetragonEventStream and forwards them
// to the pipeline. This is the primary production ingestion path.
func (s *GRPCSource) StreamEvents(ctx context.Context, stream TetragonEventStream, pipeline *Pipeline) error {
	slog.Info("tetragonaudit: starting gRPC event stream", "addr", s.addr)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		data, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				slog.Info("tetragonaudit: gRPC stream ended")
				return nil
			}
			return fmt.Errorf("tetragonaudit: gRPC recv: %w", err)
		}

		var event Event
		if err := json.Unmarshal(data, &event); err != nil {
			slog.Debug("tetragonaudit: failed to unmarshal gRPC event", "error", err)
			continue
		}

		if err := pipeline.ProcessEvent(ctx, event); err != nil {
			slog.Warn("tetragonaudit: pipeline error", "error", err)
		}
	}
}

// Close closes the gRPC connection.
func (s *GRPCSource) Close() error {
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

// Addr returns the configured gRPC address.
func (s *GRPCSource) Addr() string {
	return s.addr
}

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

// StreamEventsWithRetry streams events from the Tetragon gRPC API with
// automatic reconnection on failure using exponential backoff.
//
// On each connection failure, the method sleeps for an exponentially-growing
// duration (with ±25% jitter) before retrying. On successful reconnection
// the backoff resets to InitialDelay.
//
// The loop exits cleanly when ctx is cancelled (graceful shutdown).
func (s *GRPCSource) StreamEventsWithRetry(ctx context.Context, pipeline *Pipeline, rc RetryConfig) error {
	rc = rc.withDefaults()
	attempt := 0

	for {
		// Check for cancellation before each attempt.
		if ctx.Err() != nil {
			return ctx.Err()
		}

		stream, err := NewGRPCEventStreamAdapter(ctx, s.conn)
		if err != nil {
			pipeline.SetConnected(false)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			delay := backoff(attempt, rc)
			slog.Warn("tetragonaudit: gRPC stream creation failed, retrying",
				"error", err, "attempt", attempt+1, "backoff", delay)
			attempt++
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
				continue
			}
		}

		slog.Info("tetragonaudit: gRPC stream connected", "addr", s.addr, "attempt", attempt+1)
		pipeline.SetConnected(true)
		attempt = 0 // Reset on successful connection.

		err = s.StreamEvents(ctx, stream, pipeline)
		pipeline.SetConnected(false)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err == nil {
			// Clean EOF — server shut the stream. Reconnect after a small delay
			// to avoid a tight loop during rolling updates or config mismatches.
			slog.Info("tetragonaudit: gRPC stream ended, reconnecting", "addr", s.addr)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(rc.InitialDelay):
				continue
			}
		}

		delay := backoff(attempt, rc)
		slog.Warn("tetragonaudit: gRPC stream error, reconnecting",
			"error", err, "attempt", attempt+1, "backoff", delay)
		attempt++
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
}

// backoff computes the delay for a given attempt with ±25% jitter.
func backoff(attempt int, rc RetryConfig) time.Duration {
	d := float64(rc.InitialDelay) * math.Pow(rc.Multiplier, float64(attempt))
	if d > float64(rc.MaxDelay) {
		d = float64(rc.MaxDelay)
	}
	// Add ±25% jitter.
	jitter := d * 0.25 * (2*rand.Float64() - 1) //nolint:gosec
	return time.Duration(d + jitter)
}

// Conn returns the underlying gRPC connection for advanced usage
// (e.g. creating Tetragon-specific clients).
func (s *GRPCSource) Conn() *grpc.ClientConn {
	return s.conn
}

// protojsonMarshaler is the shared protojson marshaler used to convert
// Tetragon protobuf responses into JSON that our Event struct can parse.
// UseProtoNames ensures field names match the proto definitions (snake_case),
// which aligns with our JSON struct tags.
var protojsonMarshaler = protojson.MarshalOptions{
	UseProtoNames: true,
}

// GRPCEventStreamAdapter wraps a Tetragon FineGuidanceSensors_GetEventsClient
// to implement TetragonEventStream. It uses the official Tetragon gRPC client
// with proper protobuf marshaling, converting each GetEventsResponse into
// JSON bytes for the downstream pipeline.
type GRPCEventStreamAdapter struct {
	stream tetragon.FineGuidanceSensors_GetEventsClient
}

// NewGRPCEventStreamAdapter creates a TetragonEventStream from a gRPC connection.
// It initiates a server-streaming RPC on the Tetragon FineGuidanceSensors/GetEvents
// endpoint using the official generated gRPC client.
func NewGRPCEventStreamAdapter(ctx context.Context, conn *grpc.ClientConn) (*GRPCEventStreamAdapter, error) {
	if conn == nil {
		return nil, fmt.Errorf("tetragonaudit: gRPC connection is nil")
	}
	client := tetragon.NewFineGuidanceSensorsClient(conn)
	stream, err := client.GetEvents(ctx, &tetragon.GetEventsRequest{})
	if err != nil {
		return nil, fmt.Errorf("tetragonaudit: failed to create gRPC stream: %w", err)
	}
	return &GRPCEventStreamAdapter{stream: stream}, nil
}

// Recv blocks until the next event is available from the gRPC stream.
// It receives a protobuf GetEventsResponse and converts it to JSON bytes
// using protojson, which the pipeline can then unmarshal into our Event struct.
func (a *GRPCEventStreamAdapter) Recv() ([]byte, error) {
	if a.stream == nil {
		return nil, io.EOF
	}
	resp, err := a.stream.Recv()
	if err != nil {
		return nil, err
	}
	// Convert protobuf response to JSON for the existing pipeline.
	data, err := protojsonMarshaler.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("tetragonaudit: failed to marshal GetEventsResponse to JSON: %w", err)
	}
	return data, nil
}
