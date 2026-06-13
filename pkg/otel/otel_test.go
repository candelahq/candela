package otel_test

import (
	"context"
	"testing"

	candelaotel "github.com/candelahq/candela/pkg/otel"
)

func TestSetup(t *testing.T) {
	shutdown, err := candelaotel.Setup(context.Background(), candelaotel.Config{
		ServiceName:    "candela-test",
		ServiceVersion: "0.0.0-test",
	})
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	defer func() { _ = shutdown(context.Background()) }()
}

func TestNewProxyMetrics(t *testing.T) {
	shutdown, err := candelaotel.Setup(context.Background(), candelaotel.Config{
		ServiceName: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = shutdown(context.Background()) }()

	m, err := candelaotel.NewProxyMetrics()
	if err != nil {
		t.Fatal(err)
	}
	if m.RequestTotal == nil {
		t.Error("RequestTotal is nil")
	}
}
