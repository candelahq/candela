#[cfg(test)]
mod tests {
    use candela_types::chat::*;
    use candela_types::session::*;
    use chrono::Utc;

    use crate::proto::candela::types as proto_types;

    /// Round-trip: domain ChatEvent → proto → domain
    #[test]
    fn chat_event_chunk_round_trip() {
        let mut original = ChatEvent::default();
        original.stream_id = "stream-1".to_string();
        let mut chunk = ChunkEvent::default();
        chunk.delta = "Hello, world!".to_string();
        original.event = Some(ChatEventEvent::Chunk(chunk));

        let proto: proto_types::ChatEvent = (&original).into();
        let recovered: ChatEvent = (&proto).try_into().expect("round-trip should succeed");

        assert_eq!(recovered.stream_id, original.stream_id);
        match (&original.event, &recovered.event) {
            (Some(ChatEventEvent::Chunk(a)), Some(ChatEventEvent::Chunk(b))) => {
                assert_eq!(a.delta, b.delta);
            }
            _ => panic!("expected Chunk variant"),
        }
    }

    /// Round-trip: domain ChatEvent with ToolCall → proto → domain
    #[test]
    fn chat_event_tool_call_round_trip() {
        let mut original = ChatEvent::default();
        original.stream_id = "stream-2".to_string();
        let mut tc = ToolCallEvent::default();
        tc.call_id = "call-42".to_string();
        tc.tool = "web_search".to_string();
        tc.args = serde_json::json!({"query": "rust"})
            .as_object()
            .unwrap()
            .clone();
        tc.requires_approval = true;
        original.event = Some(ChatEventEvent::ToolCall(tc));

        let proto: proto_types::ChatEvent = (&original).into();
        let recovered: ChatEvent = (&proto).try_into().expect("round-trip should succeed");

        assert_eq!(recovered.stream_id, original.stream_id);
        match (&original.event, &recovered.event) {
            (Some(ChatEventEvent::ToolCall(a)), Some(ChatEventEvent::ToolCall(b))) => {
                assert_eq!(a.call_id, b.call_id);
                assert_eq!(a.tool, b.tool);
                assert_eq!(a.args, b.args);
                assert_eq!(a.requires_approval, b.requires_approval);
            }
            _ => panic!("expected ToolCall variant"),
        }
    }

    /// Round-trip: domain ChatEvent with DoneEvent + usage → proto → domain
    #[test]
    fn chat_event_done_with_usage_round_trip() {
        let mut original = ChatEvent::default();
        original.stream_id = "stream-3".to_string();
        let mut done = DoneEvent::default();
        let mut usage = UsageSummary::default();
        usage.prompt_tokens = 100;
        usage.completion_tokens = 50;
        usage.total_tokens = 150;
        usage.total_cost_usd = 0.003;
        usage.model = "gpt-4".to_string();
        done.usage = Some(usage);
        original.event = Some(ChatEventEvent::Done(done));

        let proto: proto_types::ChatEvent = (&original).into();
        let recovered: ChatEvent = (&proto).try_into().expect("round-trip should succeed");

        match (&original.event, &recovered.event) {
            (Some(ChatEventEvent::Done(a)), Some(ChatEventEvent::Done(b))) => {
                let ua = a.usage.as_ref().unwrap();
                let ub = b.usage.as_ref().unwrap();
                assert_eq!(ua.prompt_tokens, ub.prompt_tokens);
                assert_eq!(ua.completion_tokens, ub.completion_tokens);
                assert_eq!(ua.total_tokens, ub.total_tokens);
                assert_eq!(ua.model, ub.model);
                assert!((ua.total_cost_usd - ub.total_cost_usd).abs() < f64::EPSILON);
            }
            _ => panic!("expected Done variant"),
        }
    }

    /// Round-trip: domain ChatEvent with StatusEvent → proto → domain
    #[test]
    fn chat_event_status_round_trip() {
        let mut original = ChatEvent::default();
        original.stream_id = "stream-4".to_string();
        let mut status = StatusEvent::default();
        status.text = "Thinking...".to_string();
        status.agent = Some("planner".to_string());
        original.event = Some(ChatEventEvent::Status(status));

        let proto: proto_types::ChatEvent = (&original).into();
        let recovered: ChatEvent = (&proto).try_into().expect("round-trip should succeed");

        match (&original.event, &recovered.event) {
            (Some(ChatEventEvent::Status(a)), Some(ChatEventEvent::Status(b))) => {
                assert_eq!(a.text, b.text);
                assert_eq!(a.agent, b.agent);
            }
            _ => panic!("expected Status variant"),
        }
    }

    /// Round-trip: domain ChatEvent with ErrorEvent → proto → domain
    #[test]
    fn chat_event_error_round_trip() {
        let mut original = ChatEvent::default();
        original.stream_id = "stream-5".to_string();
        let mut err = ErrorEvent::default();
        err.message = "rate limited".to_string();
        err.code = Some("429".to_string());
        original.event = Some(ChatEventEvent::Error(err));

        let proto: proto_types::ChatEvent = (&original).into();
        let recovered: ChatEvent = (&proto).try_into().expect("round-trip should succeed");

        match (&original.event, &recovered.event) {
            (Some(ChatEventEvent::Error(a)), Some(ChatEventEvent::Error(b))) => {
                assert_eq!(a.message, b.message);
                assert_eq!(a.code, b.code);
            }
            _ => panic!("expected Error variant"),
        }
    }

    /// Round-trip: domain Session → proto → domain
    #[test]
    fn session_round_trip() {
        let mut original = Session::default();
        original.id = "sess-abc".to_string();
        original.title = "Test Session".to_string();
        original.model = "gpt-4o".to_string();
        original.message_count = 42;
        original.total_tokens = 5000;
        original.total_cost_usd = 0.15;
        original.device_id = "device-xyz".to_string();
        original.created_at = Utc::now();
        original.updated_at = Utc::now();
        original.deleted_at = None;

        let proto: proto_types::Session = (&original).into();
        let recovered: Session = (&proto).try_into().expect("round-trip should succeed");

        assert_eq!(recovered.id, original.id);
        assert_eq!(recovered.title, original.title);
        assert_eq!(recovered.model, original.model);
        assert_eq!(recovered.message_count, original.message_count);
        assert_eq!(recovered.total_tokens, original.total_tokens);
        assert!((recovered.total_cost_usd - original.total_cost_usd).abs() < f64::EPSILON);
        assert_eq!(recovered.device_id, original.device_id);
        assert_eq!(recovered.created_at, original.created_at);
        assert_eq!(recovered.deleted_at, None);
    }

    /// ChatEvent with no event variant round-trips correctly
    #[test]
    fn chat_event_none_round_trip() {
        let mut original = ChatEvent::default();
        original.stream_id = "stream-empty".to_string();
        original.event = None;

        let proto: proto_types::ChatEvent = (&original).into();
        let recovered: ChatEvent = (&proto).try_into().expect("round-trip should succeed");

        assert_eq!(recovered.stream_id, original.stream_id);
        assert!(recovered.event.is_none());
    }
}
