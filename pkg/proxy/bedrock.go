package proxy

import "fmt"

// BedrockPathRewriter rewrites request paths for AWS Bedrock's model invocation
// endpoint format: /model/{modelId}/invoke or /model/{modelId}/invoke-with-response-stream
//
// IMPORTANT: The model parameter must be a valid Bedrock Model ID as registered
// in AWS (e.g., "anthropic.claude-3-5-sonnet-20241022-v2:0"). Friendly names
// like "claude-3-5-sonnet" will not work. Configure exact Bedrock Model IDs
// in your config.yaml providers section:
//
//	providers:
//	  - name: anthropic-bedrock
//	    models:
//	      - anthropic.claude-3-5-sonnet-20241022-v2:0
//	      - anthropic.claude-3-haiku-20240307-v1:0
type BedrockPathRewriter struct{}

// RewritePath returns the Bedrock model invocation path.
func (b *BedrockPathRewriter) RewritePath(model string, streaming bool) string {
	if streaming {
		return fmt.Sprintf("/model/%s/invoke-with-response-stream", model)
	}
	return fmt.Sprintf("/model/%s/invoke", model)
}
