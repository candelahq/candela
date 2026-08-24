package connecthandlers

import (
	"errors"
	"testing"

	"connectrpc.com/connect"
)

func TestInternalError(t *testing.T) {
	origErr := errors.New("sensitive database error")
	got := internalError("something went wrong", origErr)

	if got.Code() != connect.CodeInternal {
		t.Errorf("internalError() Code = %v, want %v", got.Code(), connect.CodeInternal)
	}

	if got.Message() != "internal error" {
		t.Errorf("internalError() Message = %v, want 'internal error'", got.Message())
	}
}
