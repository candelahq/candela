package connecthandlers

import (
	"testing"

	"connectrpc.com/connect"
)

func TestExtractBaggage(t *testing.T) {
	tests := []struct {
		name    string
		baggage string
		key     string
		want    string
	}{
		{
			name:    "empty baggage",
			baggage: "",
			key:     "candela.tenant_id",
			want:    "",
		},
		{
			name:    "single item",
			baggage: "candela.tenant_id=t1",
			key:     "candela.tenant_id",
			want:    "t1",
		},
		{
			name:    "multiple items",
			baggage: "candela.tenant_id=t1,candela.job_id=j1",
			key:     "candela.job_id",
			want:    "j1",
		},
		{
			name:    "with properties",
			baggage: "candela.tenant_id=t1;foo=bar,candela.job_id=j1;baz=qux",
			key:     "candela.tenant_id",
			want:    "t1",
		},
		{
			name:    "spaces around",
			baggage: " candela.tenant_id = t1 , candela.job_id = j1 ",
			key:     "candela.job_id",
			want:    "j1",
		},
		{
			name:    "key not found",
			baggage: "foo=bar",
			key:     "candela.tenant_id",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractBaggage(tt.baggage, tt.key)
			if got != tt.want {
				t.Errorf("extractBaggage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetAttribution(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
		want    attribution
	}{
		{
			name:    "empty headers",
			headers: map[string]string{},
			want:    attribution{},
		},
		{
			name: "explicit headers",
			headers: map[string]string{
				"X-Candela-Tenant-Id": "t1",
				"X-Candela-Job-Id":    "j1",
			},
			want: attribution{TenantID: "t1", JobID: "j1"},
		},
		{
			name: "baggage fallback",
			headers: map[string]string{
				"Baggage": "candela.tenant_id=t2,candela.job_id=j2",
			},
			want: attribution{TenantID: "t2", JobID: "j2"},
		},
		{
			name: "mixed explicit and baggage",
			headers: map[string]string{
				"X-Candela-Tenant-Id": "t1",
				"Baggage":             "candela.tenant_id=t2,candela.job_id=j2",
			},
			want: attribution{TenantID: "t1", JobID: "j2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := connect.NewRequest(new(struct{}))
			for k, v := range tt.headers {
				req.Header().Set(k, v)
			}

			got := getAttribution(req)
			if got != tt.want {
				t.Errorf("getAttribution() = %v, want %v", got, tt.want)
			}
		})
	}
}
