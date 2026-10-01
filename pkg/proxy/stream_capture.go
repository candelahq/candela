package proxy

import (
	"log/slog"

	"github.com/candelahq/candela/pkg/storage"
)

// streamParseResult encapsulates all metadata extracted from a streaming response,
// including usage metrics from both head and tail buffers (#525).
type streamParseResult struct {
	content      string
	inputTokens  int64
	outputTokens int64
	cacheTokens  CacheTokens
	model        string
	isEstimated  bool // true if output tokens were estimated due to missing usage chunk
	streamStatus storage.SpanStatus
}

// extractStreamInfo reconciles token usage, model information, and output content
// across the head buffer and the tail buffer of a streaming response.
//
// When a streaming response exceeds maxStreamCapture (10MB), streamBuffer retains
// the head (for trace content and prompt metadata) while tailBuffer retains the
// most recent chunks (preserving the final usage and delta chunks).
//
// If the stream was terminated prematurely (e.g. client disconnect, upstream failure,
// or missing usage chunk), output tokens are estimated gracefully from the delivered
// text content and bytes streamed so budget deduction and billing reconciliation
// prevent revenue leakage (#525).
func (p *Proxy) extractStreamInfo(
	provider Provider,
	reqModel string,
	reqBody []byte,
	headData []byte,
	tailData []byte,
	streamCapped bool,
	streamCompleted bool,
	totalBytesStreamed int64,
	requestID string,
) streamParseResult {
	// 1. Parse head buffer (first up to 10MB).
	headContent, headInput, headOutput := extractStreamingUsage(provider.Name, headData)
	headCT := extractStreamingCacheTokens(provider.Name, headData)
	headModel := extractModelFromStreamingResponse(provider.Name, headData)

	res := streamParseResult{
		content:      headContent,
		inputTokens:  headInput,
		outputTokens: headOutput,
		cacheTokens:  headCT,
		model:        headModel,
		streamStatus: storage.SpanStatusOK,
	}

	if !streamCompleted {
		res.streamStatus = storage.SpanStatusError
	}

	// 2. If stream was capped (>10MB), inspect tail buffer for final usage chunks.
	if streamCapped && len(tailData) > 0 {
		_, tailInput, tailOutput := extractStreamingUsage(provider.Name, tailData)
		tailCT := extractStreamingCacheTokens(provider.Name, tailData)
		tailModel := extractModelFromStreamingResponse(provider.Name, tailData)

		if tailModel != "" && res.model == "" {
			res.model = tailModel
		}
		if tailOutput > 0 {
			res.outputTokens = tailOutput
		}
		if tailInput > 0 {
			res.inputTokens = tailInput
		}
		if tailCT.CacheReadTokens > res.cacheTokens.CacheReadTokens {
			res.cacheTokens.CacheReadTokens = tailCT.CacheReadTokens
		}
		if tailCT.CacheCreationTokens > res.cacheTokens.CacheCreationTokens {
			res.cacheTokens.CacheCreationTokens = tailCT.CacheCreationTokens
		}
	}

	// 3. Fallback to request model if not found in stream.
	if res.model == "" {
		res.model = reqModel
	}

	// 4. Graceful billing reconciliation for truncated or missing usage streams (#525):
	// If outputTokens is 0 but data was actually streamed to client, estimate tokens.
	if res.outputTokens == 0 && (len(res.content) > 0 || totalBytesStreamed > 0) {
		var estOutput int64
		if len(res.content) > 0 {
			// ~4 characters per token heuristic
			estOutput = int64(len(res.content)+3) / 4
		}
		// If the stream was capped, account for bytes streamed beyond maxStreamCapture
		if streamCapped && totalBytesStreamed > int64(len(headData)) {
			excessBytes := totalBytesStreamed - int64(len(headData))
			// Conservative estimate: ~20 bytes per SSE chunk framing per token
			estOutput += (excessBytes + 19) / 20
		} else if estOutput == 0 && totalBytesStreamed > 0 {
			estOutput = (totalBytesStreamed + 19) / 20
		}
		if estOutput < 1 {
			estOutput = 1
		}
		res.outputTokens = estOutput
		res.isEstimated = true

		if res.inputTokens == 0 && len(reqBody) > 0 {
			res.inputTokens = int64(len(reqBody)+3) / 4
			if res.inputTokens < 1 {
				res.inputTokens = 1
			}
		}

		slog.Warn("stream truncated or missing final usage chunk: estimated tokens for billing reconciliation",
			"provider", provider.Name,
			"request_id", requestID,
			"estimated_input_tokens", res.inputTokens,
			"estimated_output_tokens", res.outputTokens,
			"stream_completed", streamCompleted,
			"stream_capped", streamCapped,
			"total_bytes_streamed", totalBytesStreamed,
		)
	}

	return res
}
