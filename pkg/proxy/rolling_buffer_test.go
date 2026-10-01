package proxy

import (
	"bytes"
	"testing"
)

func TestRollingBuffer_EmptyWrite(t *testing.T) {
	rb := newRollingBuffer(10)
	n, err := rb.Write([]byte{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("got n=%d, want 0", n)
	}
	if rb.Len() != 0 {
		t.Errorf("got len=%d, want 0", rb.Len())
	}
	if len(rb.Bytes()) != 0 {
		t.Errorf("got bytes len=%d, want 0", len(rb.Bytes()))
	}
}

func TestRollingBuffer_UnderCapacity(t *testing.T) {
	rb := newRollingBuffer(10)
	_, _ = rb.Write([]byte("abc"))
	_, _ = rb.Write([]byte("def"))

	if rb.Len() != 6 {
		t.Errorf("got len=%d, want 6", rb.Len())
	}
	if string(rb.Bytes()) != "abcdef" {
		t.Errorf("got %q, want %q", string(rb.Bytes()), "abcdef")
	}
}

func TestRollingBuffer_ExactCapacity(t *testing.T) {
	rb := newRollingBuffer(6)
	_, _ = rb.Write([]byte("abc"))
	_, _ = rb.Write([]byte("def"))

	if rb.Len() != 6 {
		t.Errorf("got len=%d, want 6", rb.Len())
	}
	if string(rb.Bytes()) != "abcdef" {
		t.Errorf("got %q, want %q", string(rb.Bytes()), "abcdef")
	}
}

func TestRollingBuffer_SlidingWindow(t *testing.T) {
	rb := newRollingBuffer(5)
	_, _ = rb.Write([]byte("hello"))
	if string(rb.Bytes()) != "hello" {
		t.Errorf("got %q, want %q", string(rb.Bytes()), "hello")
	}

	_, _ = rb.Write([]byte("!"))
	if string(rb.Bytes()) != "ello!" {
		t.Errorf("got %q, want %q", string(rb.Bytes()), "ello!")
	}

	_, _ = rb.Write([]byte("123"))
	if string(rb.Bytes()) != "o!123" {
		t.Errorf("got %q, want %q", string(rb.Bytes()), "o!123")
	}
}

func TestRollingBuffer_WriteLargerThanMaxSize(t *testing.T) {
	rb := newRollingBuffer(5)
	_, _ = rb.Write([]byte("abcdefghijk"))

	if rb.Len() != 5 {
		t.Errorf("got len=%d, want 5", rb.Len())
	}
	if string(rb.Bytes()) != "ghijk" {
		t.Errorf("got %q, want %q", string(rb.Bytes()), "ghijk")
	}

	// Another large write overwrites completely
	_, _ = rb.Write([]byte("1234567890"))
	if string(rb.Bytes()) != "67890" {
		t.Errorf("got %q, want %q", string(rb.Bytes()), "67890")
	}
}

func TestRollingBuffer_Reset(t *testing.T) {
	rb := newRollingBuffer(5)
	_, _ = rb.Write([]byte("hello"))
	rb.Reset()
	if rb.Len() != 0 {
		t.Errorf("got len=%d, want 0 after reset", rb.Len())
	}
	_, _ = rb.Write([]byte("world"))
	if string(rb.Bytes()) != "world" {
		t.Errorf("got %q, want %q", string(rb.Bytes()), "world")
	}
}

func TestRollingBuffer_LargeChunks(t *testing.T) {
	rb := newRollingBuffer(128 * 1024)
	chunk := bytes.Repeat([]byte("A"), 64*1024)
	_, _ = rb.Write(chunk)
	_, _ = rb.Write(chunk)

	if rb.Len() != 128*1024 {
		t.Errorf("got len=%d, want 128KB", rb.Len())
	}

	finalChunk := []byte("FINAL_DATA_12345")
	_, _ = rb.Write(finalChunk)

	if rb.Len() != 128*1024 {
		t.Errorf("got len=%d, want 128KB", rb.Len())
	}
	if !bytes.HasSuffix(rb.Bytes(), finalChunk) {
		t.Errorf("buffer did not retain final chunk")
	}
}
