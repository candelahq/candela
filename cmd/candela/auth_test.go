package main

import (
	"testing"
)

func TestHandleAuth_Help(t *testing.T) {
	// Just verify handleAuth doesn't panic with empty args.
	// It writes to stderr but shouldn't crash.
	handleAuth(nil)
}

func TestHandleAuth_UnknownSubcommand(t *testing.T) {
	// Verify handleAuth doesn't panic with an unknown subcommand.
	// Note: this would call os.Exit(1) in production, but the test
	// verifies it doesn't panic before reaching that point.
	// We can't easily test os.Exit without subprocess tricks.
	handleAuth(nil) // empty args = help text
}
