package tetragonaudit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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
// without importing the full Tetragon protobuf dependency.
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

// Conn returns the underlying gRPC connection for advanced usage
// (e.g. creating Tetragon-specific clients).
func (s *GRPCSource) Conn() *grpc.ClientConn {
	return s.conn
}

// GRPCEventStreamAdapter wraps a gRPC ClientStream to implement
// TetragonEventStream. This avoids importing the full Tetragon protobuf
// dependency while still supporting real gRPC streaming.
//
// The adapter sends a GetEventsRequest on the Tetragon export RPC path
// and returns raw JSON-encoded event bytes from each response.
type GRPCEventStreamAdapter struct {
	stream grpc.ClientStream
}

// NewGRPCEventStreamAdapter creates a TetragonEventStream from a gRPC connection.
// It initiates a server-streaming RPC on the Tetragon FineGuidanceSensors/GetEvents
// endpoint and returns an adapter that reads raw event bytes.
func NewGRPCEventStreamAdapter(conn *grpc.ClientConn) *GRPCEventStreamAdapter {
	// Use a background context — the caller controls cancellation via
	// StreamEvents' context parameter.
	ctx := context.Background()
	desc := &grpc.StreamDesc{ServerStreams: true}
	stream, err := conn.NewStream(ctx, desc, "/tetragon.FineGuidanceSensors/GetEvents")
	if err != nil {
		slog.Error("tetragonaudit: failed to create gRPC stream", "error", err)
		return &GRPCEventStreamAdapter{}
	}
	// Send empty request to initiate streaming.
	if err := stream.SendMsg([]byte("{}")); err != nil {
		slog.Error("tetragonaudit: failed to send GetEvents request", "error", err)
	}
	return &GRPCEventStreamAdapter{stream: stream}
}

// Recv blocks until the next event is available from the gRPC stream.
func (a *GRPCEventStreamAdapter) Recv() ([]byte, error) {
	if a.stream == nil {
		return nil, io.EOF
	}
	var raw json.RawMessage
	if err := a.stream.RecvMsg(&raw); err != nil {
		return nil, err
	}
	return []byte(raw), nil
}
