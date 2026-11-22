//! Stream capture for SSE responses.
//!
//! The streaming path forwards bytes to the client unbuffered, so the usage and
//! cache chunks arrive after the response starts. This module tees the stream
//! into a bounded head buffer plus a rolling tail buffer and reports both when
//! the stream ends, mirroring Go's `stream_capture.go`.

use std::collections::VecDeque;
use std::pin::Pin;
use std::task::{Context, Poll};
use std::time::{Duration, Instant};

use bytes::Bytes;
use futures_core::Stream;

/// Head bytes kept for parsing. Matches Go's `maxStreamCapture`.
const MAX_HEAD: usize = 10 << 20;
/// Tail bytes kept when the head is full. Matches Go's `maxTailCapture`.
const MAX_TAIL: usize = 128 << 10;

/// Everything a stream produced, handed to the caller once the stream ends.
pub struct CapturedStream {
    pub head: Vec<u8>,
    pub tail: Vec<u8>,
    pub capped: bool,
    pub completed: bool,
    pub total_bytes: u64,
    pub ttfb: Option<Duration>,
    pub elapsed: Duration,
}

/// Tees an SSE byte stream into bounded buffers and reports the result once.
pub struct StreamTap {
    inner: Pin<Box<dyn Stream<Item = Result<Bytes, reqwest::Error>> + Send>>,
    head: Vec<u8>,
    tail: VecDeque<u8>,
    capped: bool,
    total_bytes: u64,
    ttfb: Option<Duration>,
    /// Request start, so timing matches the span rather than the first poll.
    start: Instant,
    on_finish: Option<Box<dyn FnOnce(CapturedStream) + Send>>,
}

impl StreamTap {
    pub fn new(
        start: Instant,
        inner: Pin<Box<dyn Stream<Item = Result<Bytes, reqwest::Error>> + Send>>,
        on_finish: Box<dyn FnOnce(CapturedStream) + Send>,
    ) -> Self {
        Self {
            inner,
            head: Vec::new(),
            tail: VecDeque::with_capacity(MAX_TAIL),
            capped: false,
            total_bytes: 0,
            ttfb: None,
            start,
            on_finish: Some(on_finish),
        }
    }

    fn capture(&mut self, chunk: &[u8]) {
        if self.ttfb.is_none() {
            self.ttfb = Some(self.start.elapsed());
        }
        self.total_bytes += chunk.len() as u64;

        if !self.capped {
            let room = MAX_HEAD.saturating_sub(self.head.len());
            if chunk.len() <= room {
                self.head.extend_from_slice(chunk);
            } else {
                self.head.extend_from_slice(&chunk[..room]);
                self.capped = true;
            }
        }

        if chunk.len() >= MAX_TAIL {
            self.tail.clear();
            self.tail.extend(&chunk[chunk.len() - MAX_TAIL..]);
        } else {
            let overflow = (self.tail.len() + chunk.len()).saturating_sub(MAX_TAIL);
            if overflow > 0 {
                self.tail.drain(..overflow);
            }
            self.tail.extend(chunk);
        }
    }

    fn finish(&mut self, completed: bool) {
        let Some(on_finish) = self.on_finish.take() else {
            return;
        };
        on_finish(CapturedStream {
            head: std::mem::take(&mut self.head),
            tail: self.tail.drain(..).collect(),
            capped: self.capped,
            completed,
            total_bytes: self.total_bytes,
            ttfb: self.ttfb,
            elapsed: self.start.elapsed(),
        });
    }
}

impl Stream for StreamTap {
    type Item = Result<Bytes, reqwest::Error>;

    fn poll_next(self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<Option<Self::Item>> {
        let this = self.get_mut();
        match this.inner.as_mut().poll_next(cx) {
            Poll::Ready(Some(Ok(chunk))) => {
                this.capture(&chunk);
                Poll::Ready(Some(Ok(chunk)))
            }
            Poll::Ready(None) => {
                this.finish(true);
                Poll::Ready(None)
            }
            Poll::Ready(Some(Err(e))) => {
                this.finish(false);
                Poll::Ready(Some(Err(e)))
            }
            Poll::Pending => Poll::Pending,
        }
    }
}

impl Drop for StreamTap {
    fn drop(&mut self) {
        // A client that disconnects mid-stream still gets a span.
        self.finish(false);
    }
}
