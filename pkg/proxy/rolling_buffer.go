package proxy

// rollingBuffer implements a fixed-capacity sliding buffer that retains
// the most recent maxSize bytes written to it. It is used during streaming
// response capture to ensure the final SSE/JSON chunks (containing usage
// metadata and token counts) are preserved even when the response exceeds
// the maximum stream capture threshold (#525).
type rollingBuffer struct {
	maxSize int
	buf     []byte
}

func newRollingBuffer(maxSize int) *rollingBuffer {
	return &rollingBuffer{
		maxSize: maxSize,
		buf:     make([]byte, 0, maxSize),
	}
}

// Write appends p to the rolling buffer. If len(p) exceeds the available
// capacity, older bytes are discarded to maintain a maximum of maxSize bytes.
func (rb *rollingBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if n == 0 {
		return 0, nil
	}
	if n >= rb.maxSize {
		// Incoming data alone exceeds maxSize: keep only the last maxSize bytes.
		rb.buf = append(rb.buf[:0], p[n-rb.maxSize:]...)
		return n, nil
	}

	avail := rb.maxSize - len(rb.buf)
	if n > avail {
		// Evict oldest bytes to make room.
		excess := n - avail
		copy(rb.buf, rb.buf[excess:])
		rb.buf = rb.buf[:len(rb.buf)-excess]
	}
	rb.buf = append(rb.buf, p...)
	return n, nil
}

// Bytes returns a slice of the bytes currently stored in the buffer.
func (rb *rollingBuffer) Bytes() []byte {
	return rb.buf
}

// Len returns the number of bytes currently stored in the buffer.
func (rb *rollingBuffer) Len() int {
	return len(rb.buf)
}

// Reset clears the buffer.
func (rb *rollingBuffer) Reset() {
	rb.buf = rb.buf[:0]
}
