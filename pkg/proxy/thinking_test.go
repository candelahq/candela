package proxy

import "testing"

func TestExtractThinking(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantClean string
		wantThink string
	}{
		{"no tags", "hello world", "hello world", ""},
		{"simple", "<think>reasoning here</think>answer", "answer", "reasoning here"},
		{"multiple", "<think>step 1</think>middle<think>step 2</think>end", "middleend", "step 1\nstep 2"},
		{"multiline", "<think>\nline1\nline2\n</think>result", "result", "line1\nline2"},
		{"empty think", "<think></think>content", "content", ""},
		{"nested angle brackets", "<think>x > y</think>done", "done", "x > y"},
		{"only think tags", "<think>just reasoning</think>", "", "just reasoning"},
		{"whitespace around content", "<think>  spaced  </think>  answer  ", "answer", "spaced"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clean, think := extractThinking(tt.input)
			if clean != tt.wantClean {
				t.Errorf("clean = %q, want %q", clean, tt.wantClean)
			}
			if think != tt.wantThink {
				t.Errorf("think = %q, want %q", think, tt.wantThink)
			}
		})
	}
}

func TestExtractThinking_UnclosedTag(t *testing.T) {
	clean, think := extractThinking("<think>reasoning starts here but never closes")
	if clean != "" {
		t.Errorf("clean = %q, want empty", clean)
	}
	if think != "reasoning starts here but never closes" {
		t.Errorf("think = %q, want reasoning content", think)
	}
}

func TestExtractThinking_UnclosedAfterContent(t *testing.T) {
	clean, think := extractThinking("prefix<think>reasoning")
	if clean != "prefix" {
		t.Errorf("clean = %q, want 'prefix'", clean)
	}
	if think != "reasoning" {
		t.Errorf("think = %q, want 'reasoning'", think)
	}
}

func TestIsReasoningModel(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{"deepseek-r1", true},
		{"deepseek-r1-distill-llama-70b", true},
		{"deepseek-reasoner", true},
		{"qwq-32b", true},
		{"qwen3-235b-a22b", true},
		{"gpt-4o", false},
		{"claude-sonnet-4-20250514", false},
		{"gemini-2.5-pro", false},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			if got := isReasoningModel(tt.model); got != tt.want {
				t.Errorf("isReasoningModel(%q) = %v, want %v", tt.model, got, tt.want)
			}
		})
	}
}
