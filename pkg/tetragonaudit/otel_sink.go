package tetragonaudit

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/plog/plogotlp"
)

var _ Sink = (*OTelSink)(nil)

// OTelSinkConfig holds configuration for the OTel log exporter.
type OTelSinkConfig struct {
	// Endpoint is the OTLP/HTTP endpoint (e.g. "http://localhost:4318").
	Endpoint string

	// Headers are optional auth/routing headers for the OTLP export.
	Headers map[string]string

	// TimeoutSec is the per-export HTTP timeout in seconds. Default: 10.
	TimeoutSec int
}

// OTelSink exports AuditRecords as OTLP log records via HTTP.
// It implements the Sink interface for the tetragonaudit pipeline.
type OTelSink struct {
	endpoint string
	headers  map[string]string
	client   *http.Client
}

// NewOTelSink creates a new OTel log exporter sink.
func NewOTelSink(cfg OTelSinkConfig) (*OTelSink, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("tetragonaudit: OTelSink endpoint is required")
	}
	if cfg.TimeoutSec <= 0 {
		cfg.TimeoutSec = 10
	}

	return &OTelSink{
		endpoint: cfg.Endpoint,
		headers:  cfg.Headers,
		client: &http.Client{
			Timeout: time.Duration(cfg.TimeoutSec) * time.Second,
		},
	}, nil
}

// Emit converts an AuditRecord to an OTLP LogRecord and exports it.
func (s *OTelSink) Emit(ctx context.Context, record AuditRecord) error {
	logs := BuildLogPayload(record)
	return s.exportLogs(ctx, logs)
}

// BuildLogPayload converts an AuditRecord into a plog.Logs structure.
// Exported for testing.
func BuildLogPayload(record AuditRecord) plog.Logs {
	ld := plog.NewLogs()
	rl := ld.ResourceLogs().AppendEmpty()

	// Resource attributes — identify the source.
	res := rl.Resource()
	res.Attributes().PutStr("service.name", "candela-sidecar")
	res.Attributes().PutStr("service.component", "tetragon-audit")
	if record.NodeName != "" {
		res.Attributes().PutStr("k8s.node.name", record.NodeName)
	}
	if record.Namespace != "" {
		res.Attributes().PutStr("k8s.namespace.name", record.Namespace)
	}
	if record.PodName != "" {
		res.Attributes().PutStr("k8s.pod.name", record.PodName)
	}
	if record.Container != "" {
		res.Attributes().PutStr("k8s.container.name", record.Container)
	}

	sl := rl.ScopeLogs().AppendEmpty()
	sl.Scope().SetName("candela.tetragon.audit")
	sl.Scope().SetVersion("0.1.0")

	lr := sl.LogRecords().AppendEmpty()

	// Timestamp.
	lr.SetTimestamp(pcommon.NewTimestampFromTime(record.Timestamp))
	lr.SetObservedTimestamp(pcommon.NewTimestampFromTime(time.Now()))

	// Severity.
	switch record.Severity {
	case "CRITICAL":
		lr.SetSeverityNumber(plog.SeverityNumberFatal)
		lr.SetSeverityText("CRITICAL")
	case "WARN", "WARNING":
		lr.SetSeverityNumber(plog.SeverityNumberWarn)
		lr.SetSeverityText("WARN")
	default:
		lr.SetSeverityNumber(plog.SeverityNumberInfo)
		lr.SetSeverityText("INFO")
	}

	// Body: human-readable summary.
	body := fmt.Sprintf(
		"[%s] %s %s→%s:%d (%s)",
		record.Action,
		record.Binary,
		record.SrcAddr,
		record.DstAddr,
		record.DstPort,
		record.FunctionName,
	)
	lr.Body().SetStr(body)

	// Attributes: structured audit data.
	attrs := lr.Attributes()
	attrs.PutStr("candela.audit.action", record.Action)
	attrs.PutStr("candela.audit.policy", record.PolicyName)
	attrs.PutStr("candela.audit.severity", record.Severity)

	// Process metadata.
	attrs.PutStr("process.binary", record.Binary)
	attrs.PutStr("process.arguments", record.Arguments)
	attrs.PutInt("process.uid", int64(record.UID))

	// Network metadata.
	attrs.PutStr("net.dst.addr", record.DstAddr)
	attrs.PutInt("net.dst.port", int64(record.DstPort))
	attrs.PutStr("net.src.addr", record.SrcAddr)
	attrs.PutInt("net.src.port", int64(record.SrcPort))

	// Kernel probe.
	attrs.PutStr("tetragon.function", record.FunctionName)

	return ld
}

// exportLogs sends the plog.Logs payload to the OTLP/HTTP endpoint.
func (s *OTelSink) exportLogs(ctx context.Context, logs plog.Logs) error {
	req := plogotlp.NewExportRequestFromLogs(logs)
	data, err := req.MarshalProto()
	if err != nil {
		return fmt.Errorf("tetragonaudit: marshal OTLP log: %w", err)
	}

	url := strings.TrimSuffix(s.endpoint, "/") + "/v1/logs"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("tetragonaudit: create HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/x-protobuf")
	for k, v := range s.headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("tetragonaudit: OTLP export: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("tetragonaudit: OTLP export failed: HTTP %d", resp.StatusCode)
	}

	slog.Debug("tetragonaudit: exported audit log",
		"status", resp.StatusCode,
		"endpoint", url,
	)

	return nil
}
