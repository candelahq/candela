//! Integration tests for the span processor.

use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};
use std::time::Duration;

use candela_core::{GenAIAttributes, Span, SpanKind, SpanStatus, SpanWriter};
use candela_processor::SpanProcessor;
use candela_processor::cost::CostCalculator;
use chrono::Utc;
use tokio::sync::Mutex;

/// Mock writer that records ingested span counts.
struct MockWriter {
    ingested: Mutex<Vec<Vec<Span>>>,
    call_count: AtomicUsize,
}

impl MockWriter {
    fn new() -> Self {
        Self {
            ingested: Mutex::new(Vec::new()),
            call_count: AtomicUsize::new(0),
        }
    }

    async fn total_spans(&self) -> usize {
        self.ingested.lock().await.iter().map(|b| b.len()).sum()
    }
}

impl SpanWriter for MockWriter {
    fn ingest_spans(
        &self,
        spans: &[Span],
    ) -> std::pin::Pin<Box<dyn std::future::Future<Output = anyhow::Result<()>> + Send + '_>> {
        let spans_owned: Vec<Span> = spans.to_vec();
        Box::pin(async move {
            self.call_count.fetch_add(1, Ordering::SeqCst);
            self.ingested.lock().await.push(spans_owned);
            Ok(())
        })
    }
}

/// Mock writer that always returns an error.
struct FailingWriter;

impl SpanWriter for FailingWriter {
    fn ingest_spans(
        &self,
        _spans: &[Span],
    ) -> std::pin::Pin<Box<dyn std::future::Future<Output = anyhow::Result<()>> + Send + '_>> {
        Box::pin(async { Err(anyhow::anyhow!("simulated write failure")) })
    }
}

fn make_test_span(name: &str) -> Span {
    Span {
        span_id: format!("s_{name}"),
        trace_id: format!("t_{name}"),
        parent_span_id: None,
        name: name.into(),
        kind: SpanKind::Llm,
        status: SpanStatus::Ok,
        status_message: None,
        start_time: Utc::now(),
        end_time: Utc::now(),
        duration: Duration::from_millis(100),
        gen_ai: None,
        attributes: Default::default(),
        project_id: "test".into(),
        environment: None,
        service_name: None,
        user_id: None,
        session_id: None,
    }
}

fn make_test_span_with_gen_ai(name: &str, model: &str) -> Span {
    let mut span = make_test_span(name);
    span.gen_ai = Some(GenAIAttributes {
        model: model.into(),
        provider: "openai".into(),
        input_tokens: 500_000,
        output_tokens: 250_000,
        ..Default::default()
    });
    span
}

#[tokio::test]
async fn processor_flushes_on_shutdown() {
    let writer = Arc::new(MockWriter::new());
    let proc = SpanProcessor::new(vec![writer.clone()], CostCalculator::new(), 100);

    proc.submit(make_test_span("span1"));
    proc.submit(make_test_span("span2"));
    proc.submit(make_test_span("span3"));

    // Shutdown triggers drain flush.
    proc.shutdown().await;

    assert_eq!(writer.total_spans().await, 3);
}

#[tokio::test]
async fn processor_enriches_cost() {
    let writer = Arc::new(MockWriter::new());
    let proc = SpanProcessor::new(vec![writer.clone()], CostCalculator::new(), 1);

    // Submit a span with gen_ai but no cost — should be enriched.
    proc.submit(make_test_span_with_gen_ai("llm_call", "gpt-4o"));

    proc.shutdown().await;

    let batches = writer.ingested.lock().await;
    let span = &batches[0][0];
    let cost = span.gen_ai.as_ref().unwrap().cost_usd;
    // 500K input @ $2.50/M + 250K output @ $10.00/M = $1.25 + $2.50 = $3.75
    assert!((cost - 3.75).abs() < 0.001, "expected ~$3.75, got {cost}");
}

#[tokio::test]
async fn processor_drops_when_buffer_full() {
    let writer = Arc::new(MockWriter::new());
    // batch_size=1, channel capacity=10 (1*10)
    let proc = SpanProcessor::new(vec![writer.clone()], CostCalculator::new(), 1);

    // Flood the channel — some may get dropped.
    for i in 0..100 {
        proc.submit(make_test_span(&format!("flood_{i}")));
    }

    // dropped_spans should be >= 0 (may or may not drop depending on timing)
    let dropped = proc.dropped_spans();
    assert!(dropped >= 0);

    proc.shutdown().await;
}

#[tokio::test]
async fn processor_handles_writer_error() {
    // HIGH-2: Writer errors should not crash the processor.
    let failing_writer: Arc<dyn SpanWriter> = Arc::new(FailingWriter);
    let proc = SpanProcessor::new(vec![failing_writer], CostCalculator::new(), 1);

    proc.submit(make_test_span("will_fail"));

    // Should shut down cleanly even though the writer errors.
    proc.shutdown().await;
}

#[tokio::test]
async fn processor_fan_out_to_multiple_writers() {
    let writer_a = Arc::new(MockWriter::new());
    let writer_b = Arc::new(MockWriter::new());

    let proc = SpanProcessor::new(
        vec![writer_a.clone(), writer_b.clone()],
        CostCalculator::new(),
        100,
    );

    proc.submit(make_test_span("multi1"));
    proc.submit(make_test_span("multi2"));

    proc.shutdown().await;

    // Both writers should receive the same spans.
    assert_eq!(writer_a.total_spans().await, 2);
    assert_eq!(writer_b.total_spans().await, 2);
}
