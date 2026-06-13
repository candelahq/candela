package proxy

import (
	"net/http"
	"testing"
	"time"
)

func TestNewUpstreamHTTPClient_Defaults(t *testing.T) {
	client := newUpstreamHTTPClient(Config{})
	if client.Timeout != 5*time.Minute {
		t.Errorf("Timeout = %v, want 5m", client.Timeout)
	}
	tr, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("Transport is not *http.Transport")
	}
	if tr.MaxIdleConns != 200 {
		t.Errorf("MaxIdleConns = %d, want 200", tr.MaxIdleConns)
	}
	if tr.MaxIdleConnsPerHost != 50 {
		t.Errorf("MaxIdleConnsPerHost = %d, want 50", tr.MaxIdleConnsPerHost)
	}
	if tr.MaxConnsPerHost != 100 {
		t.Errorf("MaxConnsPerHost = %d, want 100", tr.MaxConnsPerHost)
	}
	if !tr.ForceAttemptHTTP2 {
		t.Error("ForceAttemptHTTP2 should be true")
	}
	if tr.ResponseHeaderTimeout != 2*time.Minute {
		t.Errorf("ResponseHeaderTimeout = %v, want 2m", tr.ResponseHeaderTimeout)
	}
}

func TestNewUpstreamHTTPClient_ConfigOverrides(t *testing.T) {
	client := newUpstreamHTTPClient(Config{
		MaxIdleConns: 42, MaxIdleConnsPerHost: 7, MaxConnsPerHost: 15,
	})
	tr := client.Transport.(*http.Transport)
	if tr.MaxIdleConns != 42 {
		t.Errorf("MaxIdleConns = %d, want 42", tr.MaxIdleConns)
	}
	if tr.MaxIdleConnsPerHost != 7 {
		t.Errorf("MaxIdleConnsPerHost = %d, want 7", tr.MaxIdleConnsPerHost)
	}
	if tr.MaxConnsPerHost != 15 {
		t.Errorf("MaxConnsPerHost = %d, want 15", tr.MaxConnsPerHost)
	}
}

func TestNewUpstreamHTTPClient_ZeroValuesGetDefaults(t *testing.T) {
	client := newUpstreamHTTPClient(Config{MaxIdleConns: 0, MaxIdleConnsPerHost: 0, MaxConnsPerHost: 0})
	tr := client.Transport.(*http.Transport)
	if tr.MaxIdleConns != 200 {
		t.Errorf("MaxIdleConns = %d, want 200", tr.MaxIdleConns)
	}
	if tr.MaxIdleConnsPerHost != 50 {
		t.Errorf("MaxIdleConnsPerHost = %d, want 50", tr.MaxIdleConnsPerHost)
	}
	if tr.MaxConnsPerHost != 100 {
		t.Errorf("MaxConnsPerHost = %d, want 100", tr.MaxConnsPerHost)
	}
}
