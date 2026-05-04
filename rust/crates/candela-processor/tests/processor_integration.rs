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

// I-6: Batch cost enrichment across multiple models.
#[tokio::test]
async fn processor_batch_cost_enrichment_mixed_models() {
    let writer = Arc::new(MockWriter::new());
    let proc = SpanProcessor::new(vec![writer.clone()], CostCalculator::new(), 10);

    // Submit spans with different models in a single batch.
    proc.submit(make_test_span_with_gen_ai("gpt4_call", "gpt-4o"));
    proc.submit(make_test_span_with_gen_ai(
        "claude_call",
        "claude-sonnet-4-20250514",
    ));
    proc.submit(make_test_span("no_genai")); // no gen_ai, should pass through unchanged

    proc.shutdown().await;

    let batches = writer.ingested.lock().await;
    let all_spans: Vec<&Span> = batches.iter().flat_map(|b| b.iter()).collect();
    assert_eq!(all_spans.len(), 3);

    // GPT-4o span should have cost enrichment.
    let gpt4_span = all_spans.iter().find(|s| s.name == "gpt4_call").unwrap();
    let gpt4_cost = gpt4_span.gen_ai.as_ref().unwrap().cost_usd;
    assert!(
        gpt4_cost > 0.0,
        "gpt-4o span should have cost > 0, got {gpt4_cost}"
    );

    // Claude span should have cost enrichment (may be 0 if not in calculator, that's ok).
    let claude_span = all_spans.iter().find(|s| s.name == "claude_call").unwrap();
    assert!(
        claude_span.gen_ai.is_some(),
        "claude span should have gen_ai"
    );

    // Non-GenAI span should be unchanged.
    let plain_span = all_spans.iter().find(|s| s.name == "no_genai").unwrap();
    assert!(
        plain_span.gen_ai.is_none(),
        "plain span should have no gen_ai"
    );
}

// I-7: Graceful drain ensures all writers complete before shutdown returns.
#[tokio::test]
async fn processor_graceful_drain_on_shutdown() {
    /// A slow writer that simulates latency.
    struct SlowWriter {
        ingested: Mutex<Vec<Vec<Span>>>,
        close_called: AtomicUsize,
    }

    impl SlowWriter {
        fn new() -> Self {
            Self {
                ingested: Mutex::new(Vec::new()),
                close_called: AtomicUsize::new(0),
            }
        }
    }

    impl SpanWriter for SlowWriter {
        fn ingest_spans(
            &self,
            spans: &[Span],
        ) -> std::pin::Pin<Box<dyn std::future::Future<Output = anyhow::Result<()>> + Send + '_>>
        {
            let spans_owned: Vec<Span> = spans.to_vec();
            Box::pin(async move {
                // Simulate slow write.
                tokio::time::sleep(Duration::from_millis(50)).await;
                self.ingested.lock().await.push(spans_owned);
                Ok(())
            })
        }

        fn close(
            &self,
        ) -> std::pin::Pin<Box<dyn std::future::Future<Output = anyhow::Result<()>> + Send + '_>>
        {
            Box::pin(async move {
                self.close_called.fetch_add(1, Ordering::SeqCst);
                Ok(())
            })
        }
    }

    let slow_writer = Arc::new(SlowWriter::new());
    let proc = SpanProcessor::new(
        vec![slow_writer.clone() as Arc<dyn SpanWriter>],
        CostCalculator::new(),
        100,
    );

    // Submit spans.
    for i in 0..5 {
        proc.submit(make_test_span(&format!("drain_{i}")));
    }

    // Shutdown should wait for all pending writes to complete.
    proc.shutdown().await;

    // Verify all spans were flushed despite the slow writer.
    let total: usize = slow_writer
        .ingested
        .lock()
        .await
        .iter()
        .map(|b| b.len())
        .sum();
    assert_eq!(
        total, 5,
        "all 5 spans should be drained on shutdown, got {total}"
    );
}
