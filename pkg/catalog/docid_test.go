package catalog

import (
	"strings"
	"testing"
)

func TestDocID_SlashEscaping(t *testing.T) {
	// Verify slashes in provider/model names are properly escaped
	// to prevent Firestore path resolution errors.
	id := docID("huggingface", "meta-llama/Llama-3")
	if strings.Contains(id, "/") {
		t.Errorf("docID contains unescaped slash: %s", id)
	}
}
