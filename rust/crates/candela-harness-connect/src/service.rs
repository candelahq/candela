//! ConnectRPC service implementation for the harness.
//!
//! Clippy: buffa-generated types have `__buffa_unknown_fields` which makes struct
//! literal syntax noisy, and `.into()` calls are kept for clarity even when the
//! source and target types are the same String (future-proofing for Cow/ByteString).

use std::sync::{Arc, Mutex};

use buffa::MessageField;
use candela_core::harness::{self, ChatEvent as DomainChatEvent, new_session};
use candela_harness_chat::ChatRuntime;
use candela_harness_storage::{Database, SearchIndex};
use connectrpc::{
    ConnectError, RequestContext, Response, ServiceRequest, ServiceResult, ServiceStream,
};
use tracing::error;

use crate::proto::candela::harness::v1::*;

/// ConnectRPC service implementation backed by ChatRuntime + Database.
pub struct HarnessServiceImpl {
    pub chat: Arc<ChatRuntime>,
    pub db: Arc<Mutex<Database>>,
    pub search: Arc<Mutex<SearchIndex>>,
}

#[allow(
    clippy::field_reassign_with_default,
    clippy::useless_conversion,
    clippy::redundant_closure,
    clippy::manual_async_fn,
    refining_impl_trait
)]
impl HarnessService for HarnessServiceImpl {
    fn send_message(
        &self,
        _ctx: RequestContext,
        request: ServiceRequest<'_, SendMessageRequest>,
    ) -> impl std::future::Future<Output = ServiceResult<ServiceStream<ChatEvent>>> + Send {
        let session_id = request.session_id.to_string();
        let content = request.content.to_string();
        let chat = self.chat.clone();

        async move {
            let (tx, rx) = tokio::sync::mpsc::channel::<ChatEvent>(32);

            tokio::spawn(async move {
                let tx_err = tx.clone();
                let result = chat
                    .send_message(&session_id, &content, move |event| {
                        let proto_event = domain_chat_event_to_proto(&event);
                        let _ = tx.blocking_send(proto_event);
                    })
                    .await;

                if let Err(e) = result {
                    error!(?e, "send_message failed");
                    let mut err = ErrorEvent::default();
                    err.message = e.to_string().into();
                    err.code = 500;
                    let mut ce = ChatEvent::default();
                    ce.event = Some(__buffa::oneof::chat_event::Event::Error(Box::new(err)));
                    let _ = tx_err.send(ce).await;
                }
            });

            use tokio_stream::StreamExt;
            let stream = tokio_stream::wrappers::ReceiverStream::new(rx).map(Ok);
            Ok(Response::new(Box::pin(stream) as ServiceStream<_>))
        }
    }

    fn list_sessions<'a>(
        &'a self,
        _ctx: RequestContext,
        request: ServiceRequest<'_, ListSessionsRequest>,
    ) -> impl std::future::Future<
        Output = ServiceResult<impl connectrpc::Encodable<ListSessionsResponse> + Send + use<'a>>,
    > + Send {
        let limit = request.limit as i64;
        let offset = request.offset as i64;

        async move {
            let db = self
                .db
                .lock()
                .map_err(|e| ConnectError::internal(format!("lock failed: {e}")))?;
            let sessions = db
                .list_sessions(limit, offset)
                .map_err(|e| ConnectError::internal(e.to_string()))?;

            let proto_sessions: Vec<Session> =
                sessions.iter().map(domain_session_to_proto).collect();
            let total = proto_sessions.len() as i32;

            let mut resp = ListSessionsResponse::default();
            resp.sessions = proto_sessions;
            resp.total = total;
            Ok(Response::new(resp))
        }
    }

    fn create_session<'a>(
        &'a self,
        _ctx: RequestContext,
        request: ServiceRequest<'_, CreateSessionRequest>,
    ) -> impl std::future::Future<
        Output = ServiceResult<impl connectrpc::Encodable<Session> + Send + use<'a>>,
    > + Send {
        let model = request.model.to_string();
        let device_id = request.device_id.to_string();

        async move {
            let session = new_session(&model, &device_id);

            let db = self
                .db
                .lock()
                .map_err(|e| ConnectError::internal(format!("lock failed: {e}")))?;
            db.create_session(&session)
                .map_err(|e| ConnectError::internal(e.to_string()))?;

            Ok(Response::new(domain_session_to_proto(&session)))
        }
    }

    fn get_session<'a>(
        &'a self,
        _ctx: RequestContext,
        request: ServiceRequest<'_, GetSessionRequest>,
    ) -> impl std::future::Future<
        Output = ServiceResult<impl connectrpc::Encodable<Session> + Send + use<'a>>,
    > + Send {
        let session_id = request.session_id.to_string();

        async move {
            let db = self
                .db
                .lock()
                .map_err(|e| ConnectError::internal(format!("lock failed: {e}")))?;
            let session = db
                .get_session(&session_id)
                .map_err(|e| ConnectError::not_found(e.to_string()))?;

            Ok(Response::new(domain_session_to_proto(&session)))
        }
    }

    fn delete_session<'a>(
        &'a self,
        _ctx: RequestContext,
        request: ServiceRequest<'_, DeleteSessionRequest>,
    ) -> impl std::future::Future<
        Output = ServiceResult<impl connectrpc::Encodable<DeleteSessionResponse> + Send + use<'a>>,
    > + Send {
        let session_id = request.session_id.to_string();

        async move {
            let db = self
                .db
                .lock()
                .map_err(|e| ConnectError::internal(format!("lock failed: {e}")))?;
            db.delete_session(&session_id)
                .map_err(|e| ConnectError::internal(e.to_string()))?;

            Ok(Response::new(DeleteSessionResponse::default()))
        }
    }

    fn edit_session_title<'a>(
        &'a self,
        _ctx: RequestContext,
        request: ServiceRequest<'_, EditSessionTitleRequest>,
    ) -> impl std::future::Future<
        Output = ServiceResult<impl connectrpc::Encodable<Session> + Send + use<'a>>,
    > + Send {
        let session_id = request.session_id.to_string();
        let title = request.title.to_string();

        async move {
            let db = self
                .db
                .lock()
                .map_err(|e| ConnectError::internal(format!("lock failed: {e}")))?;
            db.update_session_title(&session_id, &title)
                .map_err(|e| ConnectError::internal(e.to_string()))?;
            let session = db
                .get_session(&session_id)
                .map_err(|e| ConnectError::internal(e.to_string()))?;

            Ok(Response::new(domain_session_to_proto(&session)))
        }
    }

    fn search_messages<'a>(
        &'a self,
        _ctx: RequestContext,
        request: ServiceRequest<'_, SearchMessagesRequest>,
    ) -> impl std::future::Future<
        Output = ServiceResult<impl connectrpc::Encodable<SearchMessagesResponse> + Send + use<'a>>,
    > + Send {
        let query = request.query.to_string();
        let limit = request.limit as i64;

        async move {
            let search = self
                .search
                .lock()
                .map_err(|e| ConnectError::internal(format!("lock failed: {e}")))?;
            let results = search
                .search(&query, limit)
                .map_err(|e| ConnectError::internal(e.to_string()))?;

            let proto_results: Vec<SearchResult> = results
                .iter()
                .map(|r| {
                    let mut sr = SearchResult::default();
                    sr.message_preview = r.message_preview.clone().into();
                    sr.session_id = r.session_id.clone().into();
                    sr.message_id = r.message_id.clone().into();
                    sr.session_title = r.session_title.clone().into();
                    sr.role = r.role.to_string().into();
                    sr.score = r.score;
                    sr.created_at = MessageField::some(chrono_to_buffa_timestamp(&r.created_at));
                    sr
                })
                .collect();

            let mut resp = SearchMessagesResponse::default();
            resp.results = proto_results;
            Ok(Response::new(resp))
        }
    }

    fn health<'a>(
        &'a self,
        _ctx: RequestContext,
        _request: ServiceRequest<'_, HealthRequest>,
    ) -> impl std::future::Future<
        Output = ServiceResult<impl connectrpc::Encodable<HealthResponse> + Send + use<'a>>,
    > + Send {
        async move {
            let mut resp = HealthResponse::default();
            resp.status = "ok".into();
            resp.version = env!("CARGO_PKG_VERSION").into();
            Ok(Response::new(resp))
        }
    }
}

// ── Domain ↔ Proto conversions ──
// These will be auto-generated by proto2type's buffa backend eventually.

#[allow(clippy::field_reassign_with_default, clippy::useless_conversion)]
fn domain_session_to_proto(s: &harness::Session) -> Session {
    let mut session = Session::default();
    session.id = s.id.clone().into();
    session.title = s.title.clone().into();
    session.model = s.model.clone().into();
    session.message_count = s.message_count;
    session.total_tokens = s.total_tokens;
    session.total_cost_usd = Some(s.total_cost_usd);
    session.device_id = s.device_id.clone().into();
    session.created_at = MessageField::some(chrono_to_buffa_timestamp(&s.created_at));
    session.updated_at = MessageField::some(chrono_to_buffa_timestamp(&s.updated_at));
    session.deleted_at = match &s.deleted_at {
        Some(dt) => MessageField::some(chrono_to_buffa_timestamp(dt)),
        None => MessageField::none(),
    };
    session
}

#[allow(clippy::field_reassign_with_default, clippy::useless_conversion)]
fn domain_chat_event_to_proto(event: &DomainChatEvent) -> ChatEvent {
    let mut ce = ChatEvent::default();
    ce.event = Some(match event {
        DomainChatEvent::Chunk { delta, .. } => {
            let mut e = ChunkEvent::default();
            e.content = delta.clone().into();
            __buffa::oneof::chat_event::Event::Chunk(Box::new(e))
        }
        DomainChatEvent::Done { stream_id, .. } => {
            let mut e = DoneEvent::default();
            e.message_id = stream_id.clone().into();
            __buffa::oneof::chat_event::Event::Done(Box::new(e))
        }
        DomainChatEvent::Error { message, .. } => {
            let mut e = ErrorEvent::default();
            e.message = message.clone().into();
            e.code = 500;
            __buffa::oneof::chat_event::Event::Error(Box::new(e))
        }
        DomainChatEvent::Status { text, .. } => {
            let mut e = StatusEvent::default();
            e.status = text.clone().into();
            __buffa::oneof::chat_event::Event::Status(Box::new(e))
        }
        DomainChatEvent::ToolCall {
            call_id,
            tool,
            args,
            ..
        } => {
            let mut e = ToolCallEvent::default();
            e.id = call_id.clone().into();
            e.name = tool.clone().into();
            e.arguments = args.to_string().into();
            __buffa::oneof::chat_event::Event::ToolCall(Box::new(e))
        }
    });
    ce
}

#[allow(clippy::field_reassign_with_default)]
fn chrono_to_buffa_timestamp(
    dt: &chrono::DateTime<chrono::Utc>,
) -> buffa_types::google::protobuf::Timestamp {
    let mut ts = buffa_types::google::protobuf::Timestamp::default();
    ts.seconds = dt.timestamp();
    ts.nanos = dt.timestamp_subsec_nanos() as i32;
    ts
}
