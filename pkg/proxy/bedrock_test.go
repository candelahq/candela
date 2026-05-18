package proxy

import (
	"testing"
)

func TestBedrockPathRewriter_Invoke(t *testing.T) {
	rw := &BedrockPathRewriter{}

	tests := []struct {
		model     string
		streaming bool
		want      string
	}{
		{
			model:     "anthropic.claude-3-5-sonnet-20241022-v2:0",
			streaming: false,
			want:      "/model/anthropic.claude-3-5-sonnet-20241022-v2:0/invoke",
		},
		{
			model:     "anthropic.claude-3-5-sonnet-20241022-v2:0",
			streaming: true,
			want:      "/model/anthropic.claude-3-5-sonnet-20241022-v2:0/invoke-with-response-stream",
		},
		{
			model:     "anthropic.claude-3-haiku-20240307-v1:0",
			streaming: false,
			want:      "/model/anthropic.claude-3-haiku-20240307-v1:0/invoke",
		},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := rw.RewritePath(tt.model, tt.streaming)
			if got != tt.want {
				t.Errorf("RewritePath(%q, %v) = %q, want %q", tt.model, tt.streaming, got, tt.want)
			}
		})
	}
}
