//! Batched span processor and cost calculator.
//!
//! Buffers incoming spans and flushes them to one or more [`SpanWriter`] sinks
//! in batches. Used by the sidecar (and eventually local/server) for consistent
//! span handling.
//!
//! Ported from: `pkg/processor/processor.go` + `pkg/costcalc/calculator.go`

pub mod cost;

use std::sync::Arc;
use std::sync::atomic::{AtomicI64, Ordering};
use std::time::Duration;

use candela_core::{Span, SpanWriter};
use tokio::sync::mpsc;
use tokio::task::JoinHandle;
use tracing::{debug, error, warn};

use cost::CostCalculator;

/// Batched span processor that buffers and flushes to storage sinks.
pub struct SpanProcessor {
    tx: mpsc::Sender<Span>,
    handle: JoinHandle<()>,
    dropped_spans: Arc<AtomicI64>,
}

impl SpanProcessor {
    /// Create a new span processor.
    ///
    /// All provided writers receive every batch on flush.
    pub fn new(writers: Vec<Arc<dyn SpanWriter>>, calc: CostCalculator, batch_size: usize) -> Self {
        let batch_size = if batch_size == 0 { 100 } else { batch_size };
        let (tx, rx) = mpsc::channel(batch_size * 10);
        let dropped = Arc::new(AtomicI64::new(0));
        let dropped_clone = dropped.clone();

        let handle = tokio::spawn(async move {
            run_loop(rx, writers, calc, batch_size).await;
        });

        Self {
            tx,
            handle,
            dropped_spans: dropped_clone,
        }
    }

    /// Submit a single span to the processing pipeline.
    pub fn submit(&self, span: Span) {
        if self.tx.try_send(span).is_err() {
            let dropped = self.dropped_spans.fetch_add(1, Ordering::Relaxed) + 1;
            warn!(
                total_dropped = dropped,
                "span processor buffer full, dropping span"
            );
        }
    }

    /// Submit a batch of spans.
    pub fn submit_batch(&self, spans: Vec<Span>) {
        for span in spans {
            self.submit(span);
        }
    }

    /// Returns the total number of dropped spans.
    pub fn dropped_spans(&self) -> i64 {
        self.dropped_spans.load(Ordering::Relaxed)
    }

    /// Flush pending spans and shut down the processor.
    pub async fn shutdown(self) {
        // Drop the sender to signal the run loop to drain and exit.
        drop(self.tx);
        let _ = self.handle.await;
    }
}

async fn run_loop(
    mut rx: mpsc::Receiver<Span>,
    writers: Vec<Arc<dyn SpanWriter>>,
    calc: CostCalculator,
    batch_size: usize,
) {
    let mut batch = Vec::with_capacity(batch_size);
    let mut interval = tokio::time::interval(Duration::from_secs(2));
    // The first tick completes immediately — consume it.
    interval.tick().await;

    loop {
        tokio::select! {
            // Bias toward receiving spans over timer ticks.
            biased;

            msg = rx.recv() => {
                match msg {
                    Some(span) => {
                        batch.push(span);
                        if batch.len() >= batch_size {
                            flush(&mut batch, &writers, &calc).await;
                        }
                    }
                    None => {
                        // Channel closed — drain remaining.
                        flush(&mut batch, &writers, &calc).await;
                        break;
                    }
                }
            }
            _ = interval.tick() => {
                flush(&mut batch, &writers, &calc).await;
            }
        }
    }
}

async fn flush(batch: &mut Vec<Span>, writers: &[Arc<dyn SpanWriter>], calc: &CostCalculator) {
    if batch.is_empty() {
        return;
    }

    // Enrich with cost data.
    for span in batch.iter_mut() {
        if let Some(ref mut gen_ai) = span.gen_ai
            && gen_ai.cost_usd == 0.0
        {
            gen_ai.cost_usd = calc.calculate(
                &gen_ai.provider,
                &gen_ai.model,
                gen_ai.input_tokens,
                gen_ai.output_tokens,
            );
        }
    }

    // Fan-out to all sinks in parallel.
    // Use Arc<[Span]> to share the batch across tasks without cloning —
    // avoids expensive allocations when spans contain large content strings.
    let count = batch.len();
    let shared_spans: Arc<[Span]> = Arc::from(std::mem::take(batch).into_boxed_slice());
    let mut handles = Vec::with_capacity(writers.len());
    for writer in writers {
        let spans = Arc::clone(&shared_spans);
        let writer = Arc::clone(writer);
        handles.push(tokio::spawn(async move {
            if let Err(e) = writer.ingest_spans(&spans).await {
                error!(error = %e, count = spans.len(), "failed to flush spans");
            }
        }));
    }
    for handle in handles {
        let _ = handle.await;
    }

    debug!(
        count = count,
        sinks = writers.len(),
        "flushed spans to storage"
    );
}
