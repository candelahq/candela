// @generated
impl serde::Serialize for ApiKey {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.id.is_empty() {
            len += 1;
        }
        if !self.project_id.is_empty() {
            len += 1;
        }
        if !self.name.is_empty() {
            len += 1;
        }
        if !self.key_prefix.is_empty() {
            len += 1;
        }
        if self.active {
            len += 1;
        }
        if self.created_at.is_some() {
            len += 1;
        }
        if self.expires_at.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.types.APIKey", len)?;
        if !self.id.is_empty() {
            struct_ser.serialize_field("id", &self.id)?;
        }
        if !self.project_id.is_empty() {
            struct_ser.serialize_field("projectId", &self.project_id)?;
        }
        if !self.name.is_empty() {
            struct_ser.serialize_field("name", &self.name)?;
        }
        if !self.key_prefix.is_empty() {
            struct_ser.serialize_field("keyPrefix", &self.key_prefix)?;
        }
        if self.active {
            struct_ser.serialize_field("active", &self.active)?;
        }
        if let Some(v) = self.created_at.as_ref() {
            struct_ser.serialize_field("createdAt", v)?;
        }
        if let Some(v) = self.expires_at.as_ref() {
            struct_ser.serialize_field("expiresAt", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for ApiKey {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "id",
            "project_id",
            "projectId",
            "name",
            "key_prefix",
            "keyPrefix",
            "active",
            "created_at",
            "createdAt",
            "expires_at",
            "expiresAt",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Id,
            ProjectId,
            Name,
            KeyPrefix,
            Active,
            CreatedAt,
            ExpiresAt,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "id" => Ok(GeneratedField::Id),
                            "projectId" | "project_id" => Ok(GeneratedField::ProjectId),
                            "name" => Ok(GeneratedField::Name),
                            "keyPrefix" | "key_prefix" => Ok(GeneratedField::KeyPrefix),
                            "active" => Ok(GeneratedField::Active),
                            "createdAt" | "created_at" => Ok(GeneratedField::CreatedAt),
                            "expiresAt" | "expires_at" => Ok(GeneratedField::ExpiresAt),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = ApiKey;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.types.APIKey")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<ApiKey, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut id__ = None;
                let mut project_id__ = None;
                let mut name__ = None;
                let mut key_prefix__ = None;
                let mut active__ = None;
                let mut created_at__ = None;
                let mut expires_at__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Id => {
                            if id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("id"));
                            }
                            id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ProjectId => {
                            if project_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("projectId"));
                            }
                            project_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Name => {
                            if name__.is_some() {
                                return Err(serde::de::Error::duplicate_field("name"));
                            }
                            name__ = Some(map_.next_value()?);
                        }
                        GeneratedField::KeyPrefix => {
                            if key_prefix__.is_some() {
                                return Err(serde::de::Error::duplicate_field("keyPrefix"));
                            }
                            key_prefix__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Active => {
                            if active__.is_some() {
                                return Err(serde::de::Error::duplicate_field("active"));
                            }
                            active__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CreatedAt => {
                            if created_at__.is_some() {
                                return Err(serde::de::Error::duplicate_field("createdAt"));
                            }
                            created_at__ = map_.next_value()?;
                        }
                        GeneratedField::ExpiresAt => {
                            if expires_at__.is_some() {
                                return Err(serde::de::Error::duplicate_field("expiresAt"));
                            }
                            expires_at__ = map_.next_value()?;
                        }
                    }
                }
                Ok(ApiKey {
                    id: id__.unwrap_or_default(),
                    project_id: project_id__.unwrap_or_default(),
                    name: name__.unwrap_or_default(),
                    key_prefix: key_prefix__.unwrap_or_default(),
                    active: active__.unwrap_or_default(),
                    created_at: created_at__,
                    expires_at: expires_at__,
                })
            }
        }
        deserializer.deserialize_struct("candela.types.APIKey", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for Attribute {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.key.is_empty() {
            len += 1;
        }
        if self.value.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.types.Attribute", len)?;
        if !self.key.is_empty() {
            struct_ser.serialize_field("key", &self.key)?;
        }
        if let Some(v) = self.value.as_ref() {
            match v {
                attribute::Value::StringValue(v) => {
                    struct_ser.serialize_field("stringValue", v)?;
                }
                attribute::Value::IntValue(v) => {
                    #[allow(clippy::needless_borrow)]
                    #[allow(clippy::needless_borrows_for_generic_args)]
                    struct_ser.serialize_field("intValue", ToString::to_string(&v).as_str())?;
                }
                attribute::Value::DoubleValue(v) => {
                    struct_ser.serialize_field("doubleValue", v)?;
                }
                attribute::Value::BoolValue(v) => {
                    struct_ser.serialize_field("boolValue", v)?;
                }
            }
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for Attribute {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "key",
            "string_value",
            "stringValue",
            "int_value",
            "intValue",
            "double_value",
            "doubleValue",
            "bool_value",
            "boolValue",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Key,
            StringValue,
            IntValue,
            DoubleValue,
            BoolValue,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "key" => Ok(GeneratedField::Key),
                            "stringValue" | "string_value" => Ok(GeneratedField::StringValue),
                            "intValue" | "int_value" => Ok(GeneratedField::IntValue),
                            "doubleValue" | "double_value" => Ok(GeneratedField::DoubleValue),
                            "boolValue" | "bool_value" => Ok(GeneratedField::BoolValue),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = Attribute;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.types.Attribute")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<Attribute, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut key__ = None;
                let mut value__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Key => {
                            if key__.is_some() {
                                return Err(serde::de::Error::duplicate_field("key"));
                            }
                            key__ = Some(map_.next_value()?);
                        }
                        GeneratedField::StringValue => {
                            if value__.is_some() {
                                return Err(serde::de::Error::duplicate_field("stringValue"));
                            }
                            value__ = map_.next_value::<::std::option::Option<_>>()?.map(attribute::Value::StringValue);
                        }
                        GeneratedField::IntValue => {
                            if value__.is_some() {
                                return Err(serde::de::Error::duplicate_field("intValue"));
                            }
                            value__ = map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| attribute::Value::IntValue(x.0));
                        }
                        GeneratedField::DoubleValue => {
                            if value__.is_some() {
                                return Err(serde::de::Error::duplicate_field("doubleValue"));
                            }
                            value__ = map_.next_value::<::std::option::Option<::pbjson::private::NumberDeserialize<_>>>()?.map(|x| attribute::Value::DoubleValue(x.0));
                        }
                        GeneratedField::BoolValue => {
                            if value__.is_some() {
                                return Err(serde::de::Error::duplicate_field("boolValue"));
                            }
                            value__ = map_.next_value::<::std::option::Option<_>>()?.map(attribute::Value::BoolValue);
                        }
                    }
                }
                Ok(Attribute {
                    key: key__.unwrap_or_default(),
                    value: value__,
                })
            }
        }
        deserializer.deserialize_struct("candela.types.Attribute", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for AuditEntry {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.id.is_empty() {
            len += 1;
        }
        if !self.user_id.is_empty() {
            len += 1;
        }
        if !self.actor_email.is_empty() {
            len += 1;
        }
        if !self.action.is_empty() {
            len += 1;
        }
        if !self.details.is_empty() {
            len += 1;
        }
        if self.timestamp.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.types.AuditEntry", len)?;
        if !self.id.is_empty() {
            struct_ser.serialize_field("id", &self.id)?;
        }
        if !self.user_id.is_empty() {
            struct_ser.serialize_field("userId", &self.user_id)?;
        }
        if !self.actor_email.is_empty() {
            struct_ser.serialize_field("actorEmail", &self.actor_email)?;
        }
        if !self.action.is_empty() {
            struct_ser.serialize_field("action", &self.action)?;
        }
        if !self.details.is_empty() {
            struct_ser.serialize_field("details", &self.details)?;
        }
        if let Some(v) = self.timestamp.as_ref() {
            struct_ser.serialize_field("timestamp", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for AuditEntry {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "id",
            "user_id",
            "userId",
            "actor_email",
            "actorEmail",
            "action",
            "details",
            "timestamp",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Id,
            UserId,
            ActorEmail,
            Action,
            Details,
            Timestamp,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "id" => Ok(GeneratedField::Id),
                            "userId" | "user_id" => Ok(GeneratedField::UserId),
                            "actorEmail" | "actor_email" => Ok(GeneratedField::ActorEmail),
                            "action" => Ok(GeneratedField::Action),
                            "details" => Ok(GeneratedField::Details),
                            "timestamp" => Ok(GeneratedField::Timestamp),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = AuditEntry;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.types.AuditEntry")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<AuditEntry, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut id__ = None;
                let mut user_id__ = None;
                let mut actor_email__ = None;
                let mut action__ = None;
                let mut details__ = None;
                let mut timestamp__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Id => {
                            if id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("id"));
                            }
                            id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::UserId => {
                            if user_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("userId"));
                            }
                            user_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ActorEmail => {
                            if actor_email__.is_some() {
                                return Err(serde::de::Error::duplicate_field("actorEmail"));
                            }
                            actor_email__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Action => {
                            if action__.is_some() {
                                return Err(serde::de::Error::duplicate_field("action"));
                            }
                            action__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Details => {
                            if details__.is_some() {
                                return Err(serde::de::Error::duplicate_field("details"));
                            }
                            details__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Timestamp => {
                            if timestamp__.is_some() {
                                return Err(serde::de::Error::duplicate_field("timestamp"));
                            }
                            timestamp__ = map_.next_value()?;
                        }
                    }
                }
                Ok(AuditEntry {
                    id: id__.unwrap_or_default(),
                    user_id: user_id__.unwrap_or_default(),
                    actor_email: actor_email__.unwrap_or_default(),
                    action: action__.unwrap_or_default(),
                    details: details__.unwrap_or_default(),
                    timestamp: timestamp__,
                })
            }
        }
        deserializer.deserialize_struct("candela.types.AuditEntry", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for BqAttribute {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.key.is_empty() {
            len += 1;
        }
        if !self.value.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.types.BqAttribute", len)?;
        if !self.key.is_empty() {
            struct_ser.serialize_field("key", &self.key)?;
        }
        if !self.value.is_empty() {
            struct_ser.serialize_field("value", &self.value)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for BqAttribute {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "key",
            "value",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Key,
            Value,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "key" => Ok(GeneratedField::Key),
                            "value" => Ok(GeneratedField::Value),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = BqAttribute;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.types.BqAttribute")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<BqAttribute, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut key__ = None;
                let mut value__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Key => {
                            if key__.is_some() {
                                return Err(serde::de::Error::duplicate_field("key"));
                            }
                            key__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Value => {
                            if value__.is_some() {
                                return Err(serde::de::Error::duplicate_field("value"));
                            }
                            value__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(BqAttribute {
                    key: key__.unwrap_or_default(),
                    value: value__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.types.BqAttribute", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for BqSpanRow {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.span_id.is_empty() {
            len += 1;
        }
        if !self.trace_id.is_empty() {
            len += 1;
        }
        if !self.parent_span_id.is_empty() {
            len += 1;
        }
        if !self.name.is_empty() {
            len += 1;
        }
        if self.kind != 0 {
            len += 1;
        }
        if self.status != 0 {
            len += 1;
        }
        if !self.status_message.is_empty() {
            len += 1;
        }
        if self.start_time.is_some() {
            len += 1;
        }
        if self.end_time.is_some() {
            len += 1;
        }
        if self.duration_ns != 0 {
            len += 1;
        }
        if !self.project_id.is_empty() {
            len += 1;
        }
        if !self.environment.is_empty() {
            len += 1;
        }
        if !self.service_name.is_empty() {
            len += 1;
        }
        if !self.user_id.is_empty() {
            len += 1;
        }
        if !self.session_id.is_empty() {
            len += 1;
        }
        if !self.gen_ai_model.is_empty() {
            len += 1;
        }
        if !self.gen_ai_provider.is_empty() {
            len += 1;
        }
        if self.gen_ai_input_tokens != 0 {
            len += 1;
        }
        if self.gen_ai_output_tokens != 0 {
            len += 1;
        }
        if self.gen_ai_total_tokens != 0 {
            len += 1;
        }
        if self.gen_ai_cost_usd != 0. {
            len += 1;
        }
        if self.gen_ai_temperature != 0. {
            len += 1;
        }
        if self.gen_ai_max_tokens != 0 {
            len += 1;
        }
        if !self.gen_ai_input_content.is_empty() {
            len += 1;
        }
        if !self.gen_ai_output_content.is_empty() {
            len += 1;
        }
        if !self.attributes.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.types.BqSpanRow", len)?;
        if !self.span_id.is_empty() {
            struct_ser.serialize_field("spanId", &self.span_id)?;
        }
        if !self.trace_id.is_empty() {
            struct_ser.serialize_field("traceId", &self.trace_id)?;
        }
        if !self.parent_span_id.is_empty() {
            struct_ser.serialize_field("parentSpanId", &self.parent_span_id)?;
        }
        if !self.name.is_empty() {
            struct_ser.serialize_field("name", &self.name)?;
        }
        if self.kind != 0 {
            struct_ser.serialize_field("kind", &self.kind)?;
        }
        if self.status != 0 {
            struct_ser.serialize_field("status", &self.status)?;
        }
        if !self.status_message.is_empty() {
            struct_ser.serialize_field("statusMessage", &self.status_message)?;
        }
        if let Some(v) = self.start_time.as_ref() {
            struct_ser.serialize_field("startTime", v)?;
        }
        if let Some(v) = self.end_time.as_ref() {
            struct_ser.serialize_field("endTime", v)?;
        }
        if self.duration_ns != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("durationNs", ToString::to_string(&self.duration_ns).as_str())?;
        }
        if !self.project_id.is_empty() {
            struct_ser.serialize_field("projectId", &self.project_id)?;
        }
        if !self.environment.is_empty() {
            struct_ser.serialize_field("environment", &self.environment)?;
        }
        if !self.service_name.is_empty() {
            struct_ser.serialize_field("serviceName", &self.service_name)?;
        }
        if !self.user_id.is_empty() {
            struct_ser.serialize_field("userId", &self.user_id)?;
        }
        if !self.session_id.is_empty() {
            struct_ser.serialize_field("sessionId", &self.session_id)?;
        }
        if !self.gen_ai_model.is_empty() {
            struct_ser.serialize_field("genAiModel", &self.gen_ai_model)?;
        }
        if !self.gen_ai_provider.is_empty() {
            struct_ser.serialize_field("genAiProvider", &self.gen_ai_provider)?;
        }
        if self.gen_ai_input_tokens != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("genAiInputTokens", ToString::to_string(&self.gen_ai_input_tokens).as_str())?;
        }
        if self.gen_ai_output_tokens != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("genAiOutputTokens", ToString::to_string(&self.gen_ai_output_tokens).as_str())?;
        }
        if self.gen_ai_total_tokens != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("genAiTotalTokens", ToString::to_string(&self.gen_ai_total_tokens).as_str())?;
        }
        if self.gen_ai_cost_usd != 0. {
            struct_ser.serialize_field("genAiCostUsd", &self.gen_ai_cost_usd)?;
        }
        if self.gen_ai_temperature != 0. {
            struct_ser.serialize_field("genAiTemperature", &self.gen_ai_temperature)?;
        }
        if self.gen_ai_max_tokens != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("genAiMaxTokens", ToString::to_string(&self.gen_ai_max_tokens).as_str())?;
        }
        if !self.gen_ai_input_content.is_empty() {
            struct_ser.serialize_field("genAiInputContent", &self.gen_ai_input_content)?;
        }
        if !self.gen_ai_output_content.is_empty() {
            struct_ser.serialize_field("genAiOutputContent", &self.gen_ai_output_content)?;
        }
        if !self.attributes.is_empty() {
            struct_ser.serialize_field("attributes", &self.attributes)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for BqSpanRow {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "span_id",
            "spanId",
            "trace_id",
            "traceId",
            "parent_span_id",
            "parentSpanId",
            "name",
            "kind",
            "status",
            "status_message",
            "statusMessage",
            "start_time",
            "startTime",
            "end_time",
            "endTime",
            "duration_ns",
            "durationNs",
            "project_id",
            "projectId",
            "environment",
            "service_name",
            "serviceName",
            "user_id",
            "userId",
            "session_id",
            "sessionId",
            "gen_ai_model",
            "genAiModel",
            "gen_ai_provider",
            "genAiProvider",
            "gen_ai_input_tokens",
            "genAiInputTokens",
            "gen_ai_output_tokens",
            "genAiOutputTokens",
            "gen_ai_total_tokens",
            "genAiTotalTokens",
            "gen_ai_cost_usd",
            "genAiCostUsd",
            "gen_ai_temperature",
            "genAiTemperature",
            "gen_ai_max_tokens",
            "genAiMaxTokens",
            "gen_ai_input_content",
            "genAiInputContent",
            "gen_ai_output_content",
            "genAiOutputContent",
            "attributes",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            SpanId,
            TraceId,
            ParentSpanId,
            Name,
            Kind,
            Status,
            StatusMessage,
            StartTime,
            EndTime,
            DurationNs,
            ProjectId,
            Environment,
            ServiceName,
            UserId,
            SessionId,
            GenAiModel,
            GenAiProvider,
            GenAiInputTokens,
            GenAiOutputTokens,
            GenAiTotalTokens,
            GenAiCostUsd,
            GenAiTemperature,
            GenAiMaxTokens,
            GenAiInputContent,
            GenAiOutputContent,
            Attributes,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "spanId" | "span_id" => Ok(GeneratedField::SpanId),
                            "traceId" | "trace_id" => Ok(GeneratedField::TraceId),
                            "parentSpanId" | "parent_span_id" => Ok(GeneratedField::ParentSpanId),
                            "name" => Ok(GeneratedField::Name),
                            "kind" => Ok(GeneratedField::Kind),
                            "status" => Ok(GeneratedField::Status),
                            "statusMessage" | "status_message" => Ok(GeneratedField::StatusMessage),
                            "startTime" | "start_time" => Ok(GeneratedField::StartTime),
                            "endTime" | "end_time" => Ok(GeneratedField::EndTime),
                            "durationNs" | "duration_ns" => Ok(GeneratedField::DurationNs),
                            "projectId" | "project_id" => Ok(GeneratedField::ProjectId),
                            "environment" => Ok(GeneratedField::Environment),
                            "serviceName" | "service_name" => Ok(GeneratedField::ServiceName),
                            "userId" | "user_id" => Ok(GeneratedField::UserId),
                            "sessionId" | "session_id" => Ok(GeneratedField::SessionId),
                            "genAiModel" | "gen_ai_model" => Ok(GeneratedField::GenAiModel),
                            "genAiProvider" | "gen_ai_provider" => Ok(GeneratedField::GenAiProvider),
                            "genAiInputTokens" | "gen_ai_input_tokens" => Ok(GeneratedField::GenAiInputTokens),
                            "genAiOutputTokens" | "gen_ai_output_tokens" => Ok(GeneratedField::GenAiOutputTokens),
                            "genAiTotalTokens" | "gen_ai_total_tokens" => Ok(GeneratedField::GenAiTotalTokens),
                            "genAiCostUsd" | "gen_ai_cost_usd" => Ok(GeneratedField::GenAiCostUsd),
                            "genAiTemperature" | "gen_ai_temperature" => Ok(GeneratedField::GenAiTemperature),
                            "genAiMaxTokens" | "gen_ai_max_tokens" => Ok(GeneratedField::GenAiMaxTokens),
                            "genAiInputContent" | "gen_ai_input_content" => Ok(GeneratedField::GenAiInputContent),
                            "genAiOutputContent" | "gen_ai_output_content" => Ok(GeneratedField::GenAiOutputContent),
                            "attributes" => Ok(GeneratedField::Attributes),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = BqSpanRow;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.types.BqSpanRow")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<BqSpanRow, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut span_id__ = None;
                let mut trace_id__ = None;
                let mut parent_span_id__ = None;
                let mut name__ = None;
                let mut kind__ = None;
                let mut status__ = None;
                let mut status_message__ = None;
                let mut start_time__ = None;
                let mut end_time__ = None;
                let mut duration_ns__ = None;
                let mut project_id__ = None;
                let mut environment__ = None;
                let mut service_name__ = None;
                let mut user_id__ = None;
                let mut session_id__ = None;
                let mut gen_ai_model__ = None;
                let mut gen_ai_provider__ = None;
                let mut gen_ai_input_tokens__ = None;
                let mut gen_ai_output_tokens__ = None;
                let mut gen_ai_total_tokens__ = None;
                let mut gen_ai_cost_usd__ = None;
                let mut gen_ai_temperature__ = None;
                let mut gen_ai_max_tokens__ = None;
                let mut gen_ai_input_content__ = None;
                let mut gen_ai_output_content__ = None;
                let mut attributes__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::SpanId => {
                            if span_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("spanId"));
                            }
                            span_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TraceId => {
                            if trace_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("traceId"));
                            }
                            trace_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ParentSpanId => {
                            if parent_span_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("parentSpanId"));
                            }
                            parent_span_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Name => {
                            if name__.is_some() {
                                return Err(serde::de::Error::duplicate_field("name"));
                            }
                            name__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Kind => {
                            if kind__.is_some() {
                                return Err(serde::de::Error::duplicate_field("kind"));
                            }
                            kind__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Status => {
                            if status__.is_some() {
                                return Err(serde::de::Error::duplicate_field("status"));
                            }
                            status__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::StatusMessage => {
                            if status_message__.is_some() {
                                return Err(serde::de::Error::duplicate_field("statusMessage"));
                            }
                            status_message__ = Some(map_.next_value()?);
                        }
                        GeneratedField::StartTime => {
                            if start_time__.is_some() {
                                return Err(serde::de::Error::duplicate_field("startTime"));
                            }
                            start_time__ = map_.next_value()?;
                        }
                        GeneratedField::EndTime => {
                            if end_time__.is_some() {
                                return Err(serde::de::Error::duplicate_field("endTime"));
                            }
                            end_time__ = map_.next_value()?;
                        }
                        GeneratedField::DurationNs => {
                            if duration_ns__.is_some() {
                                return Err(serde::de::Error::duplicate_field("durationNs"));
                            }
                            duration_ns__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::ProjectId => {
                            if project_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("projectId"));
                            }
                            project_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Environment => {
                            if environment__.is_some() {
                                return Err(serde::de::Error::duplicate_field("environment"));
                            }
                            environment__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ServiceName => {
                            if service_name__.is_some() {
                                return Err(serde::de::Error::duplicate_field("serviceName"));
                            }
                            service_name__ = Some(map_.next_value()?);
                        }
                        GeneratedField::UserId => {
                            if user_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("userId"));
                            }
                            user_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::SessionId => {
                            if session_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("sessionId"));
                            }
                            session_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::GenAiModel => {
                            if gen_ai_model__.is_some() {
                                return Err(serde::de::Error::duplicate_field("genAiModel"));
                            }
                            gen_ai_model__ = Some(map_.next_value()?);
                        }
                        GeneratedField::GenAiProvider => {
                            if gen_ai_provider__.is_some() {
                                return Err(serde::de::Error::duplicate_field("genAiProvider"));
                            }
                            gen_ai_provider__ = Some(map_.next_value()?);
                        }
                        GeneratedField::GenAiInputTokens => {
                            if gen_ai_input_tokens__.is_some() {
                                return Err(serde::de::Error::duplicate_field("genAiInputTokens"));
                            }
                            gen_ai_input_tokens__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::GenAiOutputTokens => {
                            if gen_ai_output_tokens__.is_some() {
                                return Err(serde::de::Error::duplicate_field("genAiOutputTokens"));
                            }
                            gen_ai_output_tokens__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::GenAiTotalTokens => {
                            if gen_ai_total_tokens__.is_some() {
                                return Err(serde::de::Error::duplicate_field("genAiTotalTokens"));
                            }
                            gen_ai_total_tokens__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::GenAiCostUsd => {
                            if gen_ai_cost_usd__.is_some() {
                                return Err(serde::de::Error::duplicate_field("genAiCostUsd"));
                            }
                            gen_ai_cost_usd__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::GenAiTemperature => {
                            if gen_ai_temperature__.is_some() {
                                return Err(serde::de::Error::duplicate_field("genAiTemperature"));
                            }
                            gen_ai_temperature__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::GenAiMaxTokens => {
                            if gen_ai_max_tokens__.is_some() {
                                return Err(serde::de::Error::duplicate_field("genAiMaxTokens"));
                            }
                            gen_ai_max_tokens__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::GenAiInputContent => {
                            if gen_ai_input_content__.is_some() {
                                return Err(serde::de::Error::duplicate_field("genAiInputContent"));
                            }
                            gen_ai_input_content__ = Some(map_.next_value()?);
                        }
                        GeneratedField::GenAiOutputContent => {
                            if gen_ai_output_content__.is_some() {
                                return Err(serde::de::Error::duplicate_field("genAiOutputContent"));
                            }
                            gen_ai_output_content__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Attributes => {
                            if attributes__.is_some() {
                                return Err(serde::de::Error::duplicate_field("attributes"));
                            }
                            attributes__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(BqSpanRow {
                    span_id: span_id__.unwrap_or_default(),
                    trace_id: trace_id__.unwrap_or_default(),
                    parent_span_id: parent_span_id__.unwrap_or_default(),
                    name: name__.unwrap_or_default(),
                    kind: kind__.unwrap_or_default(),
                    status: status__.unwrap_or_default(),
                    status_message: status_message__.unwrap_or_default(),
                    start_time: start_time__,
                    end_time: end_time__,
                    duration_ns: duration_ns__.unwrap_or_default(),
                    project_id: project_id__.unwrap_or_default(),
                    environment: environment__.unwrap_or_default(),
                    service_name: service_name__.unwrap_or_default(),
                    user_id: user_id__.unwrap_or_default(),
                    session_id: session_id__.unwrap_or_default(),
                    gen_ai_model: gen_ai_model__.unwrap_or_default(),
                    gen_ai_provider: gen_ai_provider__.unwrap_or_default(),
                    gen_ai_input_tokens: gen_ai_input_tokens__.unwrap_or_default(),
                    gen_ai_output_tokens: gen_ai_output_tokens__.unwrap_or_default(),
                    gen_ai_total_tokens: gen_ai_total_tokens__.unwrap_or_default(),
                    gen_ai_cost_usd: gen_ai_cost_usd__.unwrap_or_default(),
                    gen_ai_temperature: gen_ai_temperature__.unwrap_or_default(),
                    gen_ai_max_tokens: gen_ai_max_tokens__.unwrap_or_default(),
                    gen_ai_input_content: gen_ai_input_content__.unwrap_or_default(),
                    gen_ai_output_content: gen_ai_output_content__.unwrap_or_default(),
                    attributes: attributes__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.types.BqSpanRow", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for BudgetGrant {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.id.is_empty() {
            len += 1;
        }
        if !self.user_id.is_empty() {
            len += 1;
        }
        if self.amount_usd != 0. {
            len += 1;
        }
        if self.spent_usd != 0. {
            len += 1;
        }
        if !self.reason.is_empty() {
            len += 1;
        }
        if !self.granted_by.is_empty() {
            len += 1;
        }
        if self.starts_at.is_some() {
            len += 1;
        }
        if self.expires_at.is_some() {
            len += 1;
        }
        if self.created_at.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.types.BudgetGrant", len)?;
        if !self.id.is_empty() {
            struct_ser.serialize_field("id", &self.id)?;
        }
        if !self.user_id.is_empty() {
            struct_ser.serialize_field("userId", &self.user_id)?;
        }
        if self.amount_usd != 0. {
            struct_ser.serialize_field("amountUsd", &self.amount_usd)?;
        }
        if self.spent_usd != 0. {
            struct_ser.serialize_field("spentUsd", &self.spent_usd)?;
        }
        if !self.reason.is_empty() {
            struct_ser.serialize_field("reason", &self.reason)?;
        }
        if !self.granted_by.is_empty() {
            struct_ser.serialize_field("grantedBy", &self.granted_by)?;
        }
        if let Some(v) = self.starts_at.as_ref() {
            struct_ser.serialize_field("startsAt", v)?;
        }
        if let Some(v) = self.expires_at.as_ref() {
            struct_ser.serialize_field("expiresAt", v)?;
        }
        if let Some(v) = self.created_at.as_ref() {
            struct_ser.serialize_field("createdAt", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for BudgetGrant {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "id",
            "user_id",
            "userId",
            "amount_usd",
            "amountUsd",
            "spent_usd",
            "spentUsd",
            "reason",
            "granted_by",
            "grantedBy",
            "starts_at",
            "startsAt",
            "expires_at",
            "expiresAt",
            "created_at",
            "createdAt",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Id,
            UserId,
            AmountUsd,
            SpentUsd,
            Reason,
            GrantedBy,
            StartsAt,
            ExpiresAt,
            CreatedAt,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "id" => Ok(GeneratedField::Id),
                            "userId" | "user_id" => Ok(GeneratedField::UserId),
                            "amountUsd" | "amount_usd" => Ok(GeneratedField::AmountUsd),
                            "spentUsd" | "spent_usd" => Ok(GeneratedField::SpentUsd),
                            "reason" => Ok(GeneratedField::Reason),
                            "grantedBy" | "granted_by" => Ok(GeneratedField::GrantedBy),
                            "startsAt" | "starts_at" => Ok(GeneratedField::StartsAt),
                            "expiresAt" | "expires_at" => Ok(GeneratedField::ExpiresAt),
                            "createdAt" | "created_at" => Ok(GeneratedField::CreatedAt),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = BudgetGrant;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.types.BudgetGrant")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<BudgetGrant, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut id__ = None;
                let mut user_id__ = None;
                let mut amount_usd__ = None;
                let mut spent_usd__ = None;
                let mut reason__ = None;
                let mut granted_by__ = None;
                let mut starts_at__ = None;
                let mut expires_at__ = None;
                let mut created_at__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Id => {
                            if id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("id"));
                            }
                            id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::UserId => {
                            if user_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("userId"));
                            }
                            user_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::AmountUsd => {
                            if amount_usd__.is_some() {
                                return Err(serde::de::Error::duplicate_field("amountUsd"));
                            }
                            amount_usd__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::SpentUsd => {
                            if spent_usd__.is_some() {
                                return Err(serde::de::Error::duplicate_field("spentUsd"));
                            }
                            spent_usd__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Reason => {
                            if reason__.is_some() {
                                return Err(serde::de::Error::duplicate_field("reason"));
                            }
                            reason__ = Some(map_.next_value()?);
                        }
                        GeneratedField::GrantedBy => {
                            if granted_by__.is_some() {
                                return Err(serde::de::Error::duplicate_field("grantedBy"));
                            }
                            granted_by__ = Some(map_.next_value()?);
                        }
                        GeneratedField::StartsAt => {
                            if starts_at__.is_some() {
                                return Err(serde::de::Error::duplicate_field("startsAt"));
                            }
                            starts_at__ = map_.next_value()?;
                        }
                        GeneratedField::ExpiresAt => {
                            if expires_at__.is_some() {
                                return Err(serde::de::Error::duplicate_field("expiresAt"));
                            }
                            expires_at__ = map_.next_value()?;
                        }
                        GeneratedField::CreatedAt => {
                            if created_at__.is_some() {
                                return Err(serde::de::Error::duplicate_field("createdAt"));
                            }
                            created_at__ = map_.next_value()?;
                        }
                    }
                }
                Ok(BudgetGrant {
                    id: id__.unwrap_or_default(),
                    user_id: user_id__.unwrap_or_default(),
                    amount_usd: amount_usd__.unwrap_or_default(),
                    spent_usd: spent_usd__.unwrap_or_default(),
                    reason: reason__.unwrap_or_default(),
                    granted_by: granted_by__.unwrap_or_default(),
                    starts_at: starts_at__,
                    expires_at: expires_at__,
                    created_at: created_at__,
                })
            }
        }
        deserializer.deserialize_struct("candela.types.BudgetGrant", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for BudgetPeriod {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        let variant = match self {
            Self::Unspecified => "BUDGET_PERIOD_UNSPECIFIED",
            Self::Daily => "BUDGET_PERIOD_DAILY",
        };
        serializer.serialize_str(variant)
    }
}
impl<'de> serde::Deserialize<'de> for BudgetPeriod {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "BUDGET_PERIOD_UNSPECIFIED",
            "BUDGET_PERIOD_DAILY",
        ];

        struct GeneratedVisitor;

        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = BudgetPeriod;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                write!(formatter, "expected one of: {:?}", &FIELDS)
            }

            fn visit_i64<E>(self, v: i64) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                i32::try_from(v)
                    .ok()
                    .and_then(|x| x.try_into().ok())
                    .ok_or_else(|| {
                        serde::de::Error::invalid_value(serde::de::Unexpected::Signed(v), &self)
                    })
            }

            fn visit_u64<E>(self, v: u64) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                i32::try_from(v)
                    .ok()
                    .and_then(|x| x.try_into().ok())
                    .ok_or_else(|| {
                        serde::de::Error::invalid_value(serde::de::Unexpected::Unsigned(v), &self)
                    })
            }

            fn visit_str<E>(self, value: &str) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                match value {
                    "BUDGET_PERIOD_UNSPECIFIED" => Ok(BudgetPeriod::Unspecified),
                    "BUDGET_PERIOD_DAILY" => Ok(BudgetPeriod::Daily),
                    _ => Err(serde::de::Error::unknown_variant(value, FIELDS)),
                }
            }
        }
        deserializer.deserialize_any(GeneratedVisitor)
    }
}
impl serde::Serialize for GenAiAttributes {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.model.is_empty() {
            len += 1;
        }
        if !self.provider.is_empty() {
            len += 1;
        }
        if self.input_tokens != 0 {
            len += 1;
        }
        if self.output_tokens != 0 {
            len += 1;
        }
        if self.total_tokens != 0 {
            len += 1;
        }
        if self.cost_usd != 0. {
            len += 1;
        }
        if self.temperature != 0. {
            len += 1;
        }
        if self.max_tokens != 0 {
            len += 1;
        }
        if self.top_p != 0. {
            len += 1;
        }
        if !self.input_content.is_empty() {
            len += 1;
        }
        if !self.output_content.is_empty() {
            len += 1;
        }
        if !self.input_content_ref.is_empty() {
            len += 1;
        }
        if !self.output_content_ref.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.types.GenAIAttributes", len)?;
        if !self.model.is_empty() {
            struct_ser.serialize_field("model", &self.model)?;
        }
        if !self.provider.is_empty() {
            struct_ser.serialize_field("provider", &self.provider)?;
        }
        if self.input_tokens != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("inputTokens", ToString::to_string(&self.input_tokens).as_str())?;
        }
        if self.output_tokens != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("outputTokens", ToString::to_string(&self.output_tokens).as_str())?;
        }
        if self.total_tokens != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("totalTokens", ToString::to_string(&self.total_tokens).as_str())?;
        }
        if self.cost_usd != 0. {
            struct_ser.serialize_field("costUsd", &self.cost_usd)?;
        }
        if self.temperature != 0. {
            struct_ser.serialize_field("temperature", &self.temperature)?;
        }
        if self.max_tokens != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("maxTokens", ToString::to_string(&self.max_tokens).as_str())?;
        }
        if self.top_p != 0. {
            struct_ser.serialize_field("topP", &self.top_p)?;
        }
        if !self.input_content.is_empty() {
            struct_ser.serialize_field("inputContent", &self.input_content)?;
        }
        if !self.output_content.is_empty() {
            struct_ser.serialize_field("outputContent", &self.output_content)?;
        }
        if !self.input_content_ref.is_empty() {
            struct_ser.serialize_field("inputContentRef", &self.input_content_ref)?;
        }
        if !self.output_content_ref.is_empty() {
            struct_ser.serialize_field("outputContentRef", &self.output_content_ref)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for GenAiAttributes {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "model",
            "provider",
            "input_tokens",
            "inputTokens",
            "output_tokens",
            "outputTokens",
            "total_tokens",
            "totalTokens",
            "cost_usd",
            "costUsd",
            "temperature",
            "max_tokens",
            "maxTokens",
            "top_p",
            "topP",
            "input_content",
            "inputContent",
            "output_content",
            "outputContent",
            "input_content_ref",
            "inputContentRef",
            "output_content_ref",
            "outputContentRef",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Model,
            Provider,
            InputTokens,
            OutputTokens,
            TotalTokens,
            CostUsd,
            Temperature,
            MaxTokens,
            TopP,
            InputContent,
            OutputContent,
            InputContentRef,
            OutputContentRef,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "model" => Ok(GeneratedField::Model),
                            "provider" => Ok(GeneratedField::Provider),
                            "inputTokens" | "input_tokens" => Ok(GeneratedField::InputTokens),
                            "outputTokens" | "output_tokens" => Ok(GeneratedField::OutputTokens),
                            "totalTokens" | "total_tokens" => Ok(GeneratedField::TotalTokens),
                            "costUsd" | "cost_usd" => Ok(GeneratedField::CostUsd),
                            "temperature" => Ok(GeneratedField::Temperature),
                            "maxTokens" | "max_tokens" => Ok(GeneratedField::MaxTokens),
                            "topP" | "top_p" => Ok(GeneratedField::TopP),
                            "inputContent" | "input_content" => Ok(GeneratedField::InputContent),
                            "outputContent" | "output_content" => Ok(GeneratedField::OutputContent),
                            "inputContentRef" | "input_content_ref" => Ok(GeneratedField::InputContentRef),
                            "outputContentRef" | "output_content_ref" => Ok(GeneratedField::OutputContentRef),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = GenAiAttributes;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.types.GenAIAttributes")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<GenAiAttributes, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut model__ = None;
                let mut provider__ = None;
                let mut input_tokens__ = None;
                let mut output_tokens__ = None;
                let mut total_tokens__ = None;
                let mut cost_usd__ = None;
                let mut temperature__ = None;
                let mut max_tokens__ = None;
                let mut top_p__ = None;
                let mut input_content__ = None;
                let mut output_content__ = None;
                let mut input_content_ref__ = None;
                let mut output_content_ref__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Model => {
                            if model__.is_some() {
                                return Err(serde::de::Error::duplicate_field("model"));
                            }
                            model__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Provider => {
                            if provider__.is_some() {
                                return Err(serde::de::Error::duplicate_field("provider"));
                            }
                            provider__ = Some(map_.next_value()?);
                        }
                        GeneratedField::InputTokens => {
                            if input_tokens__.is_some() {
                                return Err(serde::de::Error::duplicate_field("inputTokens"));
                            }
                            input_tokens__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::OutputTokens => {
                            if output_tokens__.is_some() {
                                return Err(serde::de::Error::duplicate_field("outputTokens"));
                            }
                            output_tokens__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::TotalTokens => {
                            if total_tokens__.is_some() {
                                return Err(serde::de::Error::duplicate_field("totalTokens"));
                            }
                            total_tokens__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::CostUsd => {
                            if cost_usd__.is_some() {
                                return Err(serde::de::Error::duplicate_field("costUsd"));
                            }
                            cost_usd__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Temperature => {
                            if temperature__.is_some() {
                                return Err(serde::de::Error::duplicate_field("temperature"));
                            }
                            temperature__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::MaxTokens => {
                            if max_tokens__.is_some() {
                                return Err(serde::de::Error::duplicate_field("maxTokens"));
                            }
                            max_tokens__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::TopP => {
                            if top_p__.is_some() {
                                return Err(serde::de::Error::duplicate_field("topP"));
                            }
                            top_p__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::InputContent => {
                            if input_content__.is_some() {
                                return Err(serde::de::Error::duplicate_field("inputContent"));
                            }
                            input_content__ = Some(map_.next_value()?);
                        }
                        GeneratedField::OutputContent => {
                            if output_content__.is_some() {
                                return Err(serde::de::Error::duplicate_field("outputContent"));
                            }
                            output_content__ = Some(map_.next_value()?);
                        }
                        GeneratedField::InputContentRef => {
                            if input_content_ref__.is_some() {
                                return Err(serde::de::Error::duplicate_field("inputContentRef"));
                            }
                            input_content_ref__ = Some(map_.next_value()?);
                        }
                        GeneratedField::OutputContentRef => {
                            if output_content_ref__.is_some() {
                                return Err(serde::de::Error::duplicate_field("outputContentRef"));
                            }
                            output_content_ref__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(GenAiAttributes {
                    model: model__.unwrap_or_default(),
                    provider: provider__.unwrap_or_default(),
                    input_tokens: input_tokens__.unwrap_or_default(),
                    output_tokens: output_tokens__.unwrap_or_default(),
                    total_tokens: total_tokens__.unwrap_or_default(),
                    cost_usd: cost_usd__.unwrap_or_default(),
                    temperature: temperature__.unwrap_or_default(),
                    max_tokens: max_tokens__.unwrap_or_default(),
                    top_p: top_p__.unwrap_or_default(),
                    input_content: input_content__.unwrap_or_default(),
                    output_content: output_content__.unwrap_or_default(),
                    input_content_ref: input_content_ref__.unwrap_or_default(),
                    output_content_ref: output_content_ref__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.types.GenAIAttributes", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for PaginationRequest {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.page_size != 0 {
            len += 1;
        }
        if !self.page_token.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.types.PaginationRequest", len)?;
        if self.page_size != 0 {
            struct_ser.serialize_field("pageSize", &self.page_size)?;
        }
        if !self.page_token.is_empty() {
            struct_ser.serialize_field("pageToken", &self.page_token)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for PaginationRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "page_size",
            "pageSize",
            "page_token",
            "pageToken",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            PageSize,
            PageToken,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "pageSize" | "page_size" => Ok(GeneratedField::PageSize),
                            "pageToken" | "page_token" => Ok(GeneratedField::PageToken),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = PaginationRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.types.PaginationRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<PaginationRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut page_size__ = None;
                let mut page_token__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::PageSize => {
                            if page_size__.is_some() {
                                return Err(serde::de::Error::duplicate_field("pageSize"));
                            }
                            page_size__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::PageToken => {
                            if page_token__.is_some() {
                                return Err(serde::de::Error::duplicate_field("pageToken"));
                            }
                            page_token__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(PaginationRequest {
                    page_size: page_size__.unwrap_or_default(),
                    page_token: page_token__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.types.PaginationRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for PaginationResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.next_page_token.is_empty() {
            len += 1;
        }
        if self.total_count != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.types.PaginationResponse", len)?;
        if !self.next_page_token.is_empty() {
            struct_ser.serialize_field("nextPageToken", &self.next_page_token)?;
        }
        if self.total_count != 0 {
            struct_ser.serialize_field("totalCount", &self.total_count)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for PaginationResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "next_page_token",
            "nextPageToken",
            "total_count",
            "totalCount",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            NextPageToken,
            TotalCount,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "nextPageToken" | "next_page_token" => Ok(GeneratedField::NextPageToken),
                            "totalCount" | "total_count" => Ok(GeneratedField::TotalCount),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = PaginationResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.types.PaginationResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<PaginationResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut next_page_token__ = None;
                let mut total_count__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::NextPageToken => {
                            if next_page_token__.is_some() {
                                return Err(serde::de::Error::duplicate_field("nextPageToken"));
                            }
                            next_page_token__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TotalCount => {
                            if total_count__.is_some() {
                                return Err(serde::de::Error::duplicate_field("totalCount"));
                            }
                            total_count__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(PaginationResponse {
                    next_page_token: next_page_token__.unwrap_or_default(),
                    total_count: total_count__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.types.PaginationResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for Project {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.id.is_empty() {
            len += 1;
        }
        if !self.name.is_empty() {
            len += 1;
        }
        if !self.description.is_empty() {
            len += 1;
        }
        if self.created_at.is_some() {
            len += 1;
        }
        if self.updated_at.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.types.Project", len)?;
        if !self.id.is_empty() {
            struct_ser.serialize_field("id", &self.id)?;
        }
        if !self.name.is_empty() {
            struct_ser.serialize_field("name", &self.name)?;
        }
        if !self.description.is_empty() {
            struct_ser.serialize_field("description", &self.description)?;
        }
        if let Some(v) = self.created_at.as_ref() {
            struct_ser.serialize_field("createdAt", v)?;
        }
        if let Some(v) = self.updated_at.as_ref() {
            struct_ser.serialize_field("updatedAt", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for Project {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "id",
            "name",
            "description",
            "created_at",
            "createdAt",
            "updated_at",
            "updatedAt",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Id,
            Name,
            Description,
            CreatedAt,
            UpdatedAt,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "id" => Ok(GeneratedField::Id),
                            "name" => Ok(GeneratedField::Name),
                            "description" => Ok(GeneratedField::Description),
                            "createdAt" | "created_at" => Ok(GeneratedField::CreatedAt),
                            "updatedAt" | "updated_at" => Ok(GeneratedField::UpdatedAt),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = Project;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.types.Project")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<Project, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut id__ = None;
                let mut name__ = None;
                let mut description__ = None;
                let mut created_at__ = None;
                let mut updated_at__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Id => {
                            if id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("id"));
                            }
                            id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Name => {
                            if name__.is_some() {
                                return Err(serde::de::Error::duplicate_field("name"));
                            }
                            name__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Description => {
                            if description__.is_some() {
                                return Err(serde::de::Error::duplicate_field("description"));
                            }
                            description__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CreatedAt => {
                            if created_at__.is_some() {
                                return Err(serde::de::Error::duplicate_field("createdAt"));
                            }
                            created_at__ = map_.next_value()?;
                        }
                        GeneratedField::UpdatedAt => {
                            if updated_at__.is_some() {
                                return Err(serde::de::Error::duplicate_field("updatedAt"));
                            }
                            updated_at__ = map_.next_value()?;
                        }
                    }
                }
                Ok(Project {
                    id: id__.unwrap_or_default(),
                    name: name__.unwrap_or_default(),
                    description: description__.unwrap_or_default(),
                    created_at: created_at__,
                    updated_at: updated_at__,
                })
            }
        }
        deserializer.deserialize_struct("candela.types.Project", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for RateWindow {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.user_id.is_empty() {
            len += 1;
        }
        if self.request_count != 0 {
            len += 1;
        }
        if self.limit != 0 {
            len += 1;
        }
        if !self.window_key.is_empty() {
            len += 1;
        }
        if self.expire_at.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.types.RateWindow", len)?;
        if !self.user_id.is_empty() {
            struct_ser.serialize_field("userId", &self.user_id)?;
        }
        if self.request_count != 0 {
            struct_ser.serialize_field("requestCount", &self.request_count)?;
        }
        if self.limit != 0 {
            struct_ser.serialize_field("limit", &self.limit)?;
        }
        if !self.window_key.is_empty() {
            struct_ser.serialize_field("windowKey", &self.window_key)?;
        }
        if let Some(v) = self.expire_at.as_ref() {
            struct_ser.serialize_field("expireAt", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for RateWindow {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "user_id",
            "userId",
            "request_count",
            "requestCount",
            "limit",
            "window_key",
            "windowKey",
            "expire_at",
            "expireAt",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            UserId,
            RequestCount,
            Limit,
            WindowKey,
            ExpireAt,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "userId" | "user_id" => Ok(GeneratedField::UserId),
                            "requestCount" | "request_count" => Ok(GeneratedField::RequestCount),
                            "limit" => Ok(GeneratedField::Limit),
                            "windowKey" | "window_key" => Ok(GeneratedField::WindowKey),
                            "expireAt" | "expire_at" => Ok(GeneratedField::ExpireAt),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = RateWindow;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.types.RateWindow")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<RateWindow, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut user_id__ = None;
                let mut request_count__ = None;
                let mut limit__ = None;
                let mut window_key__ = None;
                let mut expire_at__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::UserId => {
                            if user_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("userId"));
                            }
                            user_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::RequestCount => {
                            if request_count__.is_some() {
                                return Err(serde::de::Error::duplicate_field("requestCount"));
                            }
                            request_count__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Limit => {
                            if limit__.is_some() {
                                return Err(serde::de::Error::duplicate_field("limit"));
                            }
                            limit__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::WindowKey => {
                            if window_key__.is_some() {
                                return Err(serde::de::Error::duplicate_field("windowKey"));
                            }
                            window_key__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ExpireAt => {
                            if expire_at__.is_some() {
                                return Err(serde::de::Error::duplicate_field("expireAt"));
                            }
                            expire_at__ = map_.next_value()?;
                        }
                    }
                }
                Ok(RateWindow {
                    user_id: user_id__.unwrap_or_default(),
                    request_count: request_count__.unwrap_or_default(),
                    limit: limit__.unwrap_or_default(),
                    window_key: window_key__.unwrap_or_default(),
                    expire_at: expire_at__,
                })
            }
        }
        deserializer.deserialize_struct("candela.types.RateWindow", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for Span {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.span_id.is_empty() {
            len += 1;
        }
        if !self.trace_id.is_empty() {
            len += 1;
        }
        if !self.parent_span_id.is_empty() {
            len += 1;
        }
        if !self.name.is_empty() {
            len += 1;
        }
        if self.kind != 0 {
            len += 1;
        }
        if self.status != 0 {
            len += 1;
        }
        if !self.status_message.is_empty() {
            len += 1;
        }
        if self.start_time.is_some() {
            len += 1;
        }
        if self.end_time.is_some() {
            len += 1;
        }
        if self.duration.is_some() {
            len += 1;
        }
        if self.gen_ai.is_some() {
            len += 1;
        }
        if self.tool.is_some() {
            len += 1;
        }
        if !self.attributes.is_empty() {
            len += 1;
        }
        if !self.project_id.is_empty() {
            len += 1;
        }
        if !self.environment.is_empty() {
            len += 1;
        }
        if !self.service_name.is_empty() {
            len += 1;
        }
        if !self.user_id.is_empty() {
            len += 1;
        }
        if !self.events.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.types.Span", len)?;
        if !self.span_id.is_empty() {
            struct_ser.serialize_field("spanId", &self.span_id)?;
        }
        if !self.trace_id.is_empty() {
            struct_ser.serialize_field("traceId", &self.trace_id)?;
        }
        if !self.parent_span_id.is_empty() {
            struct_ser.serialize_field("parentSpanId", &self.parent_span_id)?;
        }
        if !self.name.is_empty() {
            struct_ser.serialize_field("name", &self.name)?;
        }
        if self.kind != 0 {
            let v = SpanKind::try_from(self.kind)
                .map_err(|_| serde::ser::Error::custom(format!("Invalid variant {}", self.kind)))?;
            struct_ser.serialize_field("kind", &v)?;
        }
        if self.status != 0 {
            let v = SpanStatus::try_from(self.status)
                .map_err(|_| serde::ser::Error::custom(format!("Invalid variant {}", self.status)))?;
            struct_ser.serialize_field("status", &v)?;
        }
        if !self.status_message.is_empty() {
            struct_ser.serialize_field("statusMessage", &self.status_message)?;
        }
        if let Some(v) = self.start_time.as_ref() {
            struct_ser.serialize_field("startTime", v)?;
        }
        if let Some(v) = self.end_time.as_ref() {
            struct_ser.serialize_field("endTime", v)?;
        }
        if let Some(v) = self.duration.as_ref() {
            struct_ser.serialize_field("duration", v)?;
        }
        if let Some(v) = self.gen_ai.as_ref() {
            struct_ser.serialize_field("genAi", v)?;
        }
        if let Some(v) = self.tool.as_ref() {
            struct_ser.serialize_field("tool", v)?;
        }
        if !self.attributes.is_empty() {
            struct_ser.serialize_field("attributes", &self.attributes)?;
        }
        if !self.project_id.is_empty() {
            struct_ser.serialize_field("projectId", &self.project_id)?;
        }
        if !self.environment.is_empty() {
            struct_ser.serialize_field("environment", &self.environment)?;
        }
        if !self.service_name.is_empty() {
            struct_ser.serialize_field("serviceName", &self.service_name)?;
        }
        if !self.user_id.is_empty() {
            struct_ser.serialize_field("userId", &self.user_id)?;
        }
        if !self.events.is_empty() {
            struct_ser.serialize_field("events", &self.events)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for Span {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "span_id",
            "spanId",
            "trace_id",
            "traceId",
            "parent_span_id",
            "parentSpanId",
            "name",
            "kind",
            "status",
            "status_message",
            "statusMessage",
            "start_time",
            "startTime",
            "end_time",
            "endTime",
            "duration",
            "gen_ai",
            "genAi",
            "tool",
            "attributes",
            "project_id",
            "projectId",
            "environment",
            "service_name",
            "serviceName",
            "user_id",
            "userId",
            "events",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            SpanId,
            TraceId,
            ParentSpanId,
            Name,
            Kind,
            Status,
            StatusMessage,
            StartTime,
            EndTime,
            Duration,
            GenAi,
            Tool,
            Attributes,
            ProjectId,
            Environment,
            ServiceName,
            UserId,
            Events,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "spanId" | "span_id" => Ok(GeneratedField::SpanId),
                            "traceId" | "trace_id" => Ok(GeneratedField::TraceId),
                            "parentSpanId" | "parent_span_id" => Ok(GeneratedField::ParentSpanId),
                            "name" => Ok(GeneratedField::Name),
                            "kind" => Ok(GeneratedField::Kind),
                            "status" => Ok(GeneratedField::Status),
                            "statusMessage" | "status_message" => Ok(GeneratedField::StatusMessage),
                            "startTime" | "start_time" => Ok(GeneratedField::StartTime),
                            "endTime" | "end_time" => Ok(GeneratedField::EndTime),
                            "duration" => Ok(GeneratedField::Duration),
                            "genAi" | "gen_ai" => Ok(GeneratedField::GenAi),
                            "tool" => Ok(GeneratedField::Tool),
                            "attributes" => Ok(GeneratedField::Attributes),
                            "projectId" | "project_id" => Ok(GeneratedField::ProjectId),
                            "environment" => Ok(GeneratedField::Environment),
                            "serviceName" | "service_name" => Ok(GeneratedField::ServiceName),
                            "userId" | "user_id" => Ok(GeneratedField::UserId),
                            "events" => Ok(GeneratedField::Events),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = Span;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.types.Span")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<Span, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut span_id__ = None;
                let mut trace_id__ = None;
                let mut parent_span_id__ = None;
                let mut name__ = None;
                let mut kind__ = None;
                let mut status__ = None;
                let mut status_message__ = None;
                let mut start_time__ = None;
                let mut end_time__ = None;
                let mut duration__ = None;
                let mut gen_ai__ = None;
                let mut tool__ = None;
                let mut attributes__ = None;
                let mut project_id__ = None;
                let mut environment__ = None;
                let mut service_name__ = None;
                let mut user_id__ = None;
                let mut events__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::SpanId => {
                            if span_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("spanId"));
                            }
                            span_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TraceId => {
                            if trace_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("traceId"));
                            }
                            trace_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ParentSpanId => {
                            if parent_span_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("parentSpanId"));
                            }
                            parent_span_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Name => {
                            if name__.is_some() {
                                return Err(serde::de::Error::duplicate_field("name"));
                            }
                            name__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Kind => {
                            if kind__.is_some() {
                                return Err(serde::de::Error::duplicate_field("kind"));
                            }
                            kind__ = Some(map_.next_value::<SpanKind>()? as i32);
                        }
                        GeneratedField::Status => {
                            if status__.is_some() {
                                return Err(serde::de::Error::duplicate_field("status"));
                            }
                            status__ = Some(map_.next_value::<SpanStatus>()? as i32);
                        }
                        GeneratedField::StatusMessage => {
                            if status_message__.is_some() {
                                return Err(serde::de::Error::duplicate_field("statusMessage"));
                            }
                            status_message__ = Some(map_.next_value()?);
                        }
                        GeneratedField::StartTime => {
                            if start_time__.is_some() {
                                return Err(serde::de::Error::duplicate_field("startTime"));
                            }
                            start_time__ = map_.next_value()?;
                        }
                        GeneratedField::EndTime => {
                            if end_time__.is_some() {
                                return Err(serde::de::Error::duplicate_field("endTime"));
                            }
                            end_time__ = map_.next_value()?;
                        }
                        GeneratedField::Duration => {
                            if duration__.is_some() {
                                return Err(serde::de::Error::duplicate_field("duration"));
                            }
                            duration__ = map_.next_value()?;
                        }
                        GeneratedField::GenAi => {
                            if gen_ai__.is_some() {
                                return Err(serde::de::Error::duplicate_field("genAi"));
                            }
                            gen_ai__ = map_.next_value()?;
                        }
                        GeneratedField::Tool => {
                            if tool__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tool"));
                            }
                            tool__ = map_.next_value()?;
                        }
                        GeneratedField::Attributes => {
                            if attributes__.is_some() {
                                return Err(serde::de::Error::duplicate_field("attributes"));
                            }
                            attributes__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ProjectId => {
                            if project_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("projectId"));
                            }
                            project_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Environment => {
                            if environment__.is_some() {
                                return Err(serde::de::Error::duplicate_field("environment"));
                            }
                            environment__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ServiceName => {
                            if service_name__.is_some() {
                                return Err(serde::de::Error::duplicate_field("serviceName"));
                            }
                            service_name__ = Some(map_.next_value()?);
                        }
                        GeneratedField::UserId => {
                            if user_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("userId"));
                            }
                            user_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Events => {
                            if events__.is_some() {
                                return Err(serde::de::Error::duplicate_field("events"));
                            }
                            events__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(Span {
                    span_id: span_id__.unwrap_or_default(),
                    trace_id: trace_id__.unwrap_or_default(),
                    parent_span_id: parent_span_id__.unwrap_or_default(),
                    name: name__.unwrap_or_default(),
                    kind: kind__.unwrap_or_default(),
                    status: status__.unwrap_or_default(),
                    status_message: status_message__.unwrap_or_default(),
                    start_time: start_time__,
                    end_time: end_time__,
                    duration: duration__,
                    gen_ai: gen_ai__,
                    tool: tool__,
                    attributes: attributes__.unwrap_or_default(),
                    project_id: project_id__.unwrap_or_default(),
                    environment: environment__.unwrap_or_default(),
                    service_name: service_name__.unwrap_or_default(),
                    user_id: user_id__.unwrap_or_default(),
                    events: events__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.types.Span", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for SpanEvent {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.name.is_empty() {
            len += 1;
        }
        if self.timestamp.is_some() {
            len += 1;
        }
        if !self.attributes.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.types.SpanEvent", len)?;
        if !self.name.is_empty() {
            struct_ser.serialize_field("name", &self.name)?;
        }
        if let Some(v) = self.timestamp.as_ref() {
            struct_ser.serialize_field("timestamp", v)?;
        }
        if !self.attributes.is_empty() {
            struct_ser.serialize_field("attributes", &self.attributes)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for SpanEvent {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "name",
            "timestamp",
            "attributes",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Name,
            Timestamp,
            Attributes,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "name" => Ok(GeneratedField::Name),
                            "timestamp" => Ok(GeneratedField::Timestamp),
                            "attributes" => Ok(GeneratedField::Attributes),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = SpanEvent;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.types.SpanEvent")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<SpanEvent, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut name__ = None;
                let mut timestamp__ = None;
                let mut attributes__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Name => {
                            if name__.is_some() {
                                return Err(serde::de::Error::duplicate_field("name"));
                            }
                            name__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Timestamp => {
                            if timestamp__.is_some() {
                                return Err(serde::de::Error::duplicate_field("timestamp"));
                            }
                            timestamp__ = map_.next_value()?;
                        }
                        GeneratedField::Attributes => {
                            if attributes__.is_some() {
                                return Err(serde::de::Error::duplicate_field("attributes"));
                            }
                            attributes__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(SpanEvent {
                    name: name__.unwrap_or_default(),
                    timestamp: timestamp__,
                    attributes: attributes__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.types.SpanEvent", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for SpanKind {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        let variant = match self {
            Self::Unspecified => "SPAN_KIND_UNSPECIFIED",
            Self::Llm => "SPAN_KIND_LLM",
            Self::Agent => "SPAN_KIND_AGENT",
            Self::Tool => "SPAN_KIND_TOOL",
            Self::Retrieval => "SPAN_KIND_RETRIEVAL",
            Self::Embedding => "SPAN_KIND_EMBEDDING",
            Self::Chain => "SPAN_KIND_CHAIN",
            Self::General => "SPAN_KIND_GENERAL",
        };
        serializer.serialize_str(variant)
    }
}
impl<'de> serde::Deserialize<'de> for SpanKind {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "SPAN_KIND_UNSPECIFIED",
            "SPAN_KIND_LLM",
            "SPAN_KIND_AGENT",
            "SPAN_KIND_TOOL",
            "SPAN_KIND_RETRIEVAL",
            "SPAN_KIND_EMBEDDING",
            "SPAN_KIND_CHAIN",
            "SPAN_KIND_GENERAL",
        ];

        struct GeneratedVisitor;

        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = SpanKind;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                write!(formatter, "expected one of: {:?}", &FIELDS)
            }

            fn visit_i64<E>(self, v: i64) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                i32::try_from(v)
                    .ok()
                    .and_then(|x| x.try_into().ok())
                    .ok_or_else(|| {
                        serde::de::Error::invalid_value(serde::de::Unexpected::Signed(v), &self)
                    })
            }

            fn visit_u64<E>(self, v: u64) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                i32::try_from(v)
                    .ok()
                    .and_then(|x| x.try_into().ok())
                    .ok_or_else(|| {
                        serde::de::Error::invalid_value(serde::de::Unexpected::Unsigned(v), &self)
                    })
            }

            fn visit_str<E>(self, value: &str) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                match value {
                    "SPAN_KIND_UNSPECIFIED" => Ok(SpanKind::Unspecified),
                    "SPAN_KIND_LLM" => Ok(SpanKind::Llm),
                    "SPAN_KIND_AGENT" => Ok(SpanKind::Agent),
                    "SPAN_KIND_TOOL" => Ok(SpanKind::Tool),
                    "SPAN_KIND_RETRIEVAL" => Ok(SpanKind::Retrieval),
                    "SPAN_KIND_EMBEDDING" => Ok(SpanKind::Embedding),
                    "SPAN_KIND_CHAIN" => Ok(SpanKind::Chain),
                    "SPAN_KIND_GENERAL" => Ok(SpanKind::General),
                    _ => Err(serde::de::Error::unknown_variant(value, FIELDS)),
                }
            }
        }
        deserializer.deserialize_any(GeneratedVisitor)
    }
}
impl serde::Serialize for SpanStatus {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        let variant = match self {
            Self::Unspecified => "SPAN_STATUS_UNSPECIFIED",
            Self::Ok => "SPAN_STATUS_OK",
            Self::Error => "SPAN_STATUS_ERROR",
        };
        serializer.serialize_str(variant)
    }
}
impl<'de> serde::Deserialize<'de> for SpanStatus {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "SPAN_STATUS_UNSPECIFIED",
            "SPAN_STATUS_OK",
            "SPAN_STATUS_ERROR",
        ];

        struct GeneratedVisitor;

        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = SpanStatus;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                write!(formatter, "expected one of: {:?}", &FIELDS)
            }

            fn visit_i64<E>(self, v: i64) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                i32::try_from(v)
                    .ok()
                    .and_then(|x| x.try_into().ok())
                    .ok_or_else(|| {
                        serde::de::Error::invalid_value(serde::de::Unexpected::Signed(v), &self)
                    })
            }

            fn visit_u64<E>(self, v: u64) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                i32::try_from(v)
                    .ok()
                    .and_then(|x| x.try_into().ok())
                    .ok_or_else(|| {
                        serde::de::Error::invalid_value(serde::de::Unexpected::Unsigned(v), &self)
                    })
            }

            fn visit_str<E>(self, value: &str) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                match value {
                    "SPAN_STATUS_UNSPECIFIED" => Ok(SpanStatus::Unspecified),
                    "SPAN_STATUS_OK" => Ok(SpanStatus::Ok),
                    "SPAN_STATUS_ERROR" => Ok(SpanStatus::Error),
                    _ => Err(serde::de::Error::unknown_variant(value, FIELDS)),
                }
            }
        }
        deserializer.deserialize_any(GeneratedVisitor)
    }
}
impl serde::Serialize for TimeRange {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.start.is_some() {
            len += 1;
        }
        if self.end.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.types.TimeRange", len)?;
        if let Some(v) = self.start.as_ref() {
            struct_ser.serialize_field("start", v)?;
        }
        if let Some(v) = self.end.as_ref() {
            struct_ser.serialize_field("end", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for TimeRange {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "start",
            "end",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Start,
            End,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "start" => Ok(GeneratedField::Start),
                            "end" => Ok(GeneratedField::End),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = TimeRange;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.types.TimeRange")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<TimeRange, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut start__ = None;
                let mut end__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Start => {
                            if start__.is_some() {
                                return Err(serde::de::Error::duplicate_field("start"));
                            }
                            start__ = map_.next_value()?;
                        }
                        GeneratedField::End => {
                            if end__.is_some() {
                                return Err(serde::de::Error::duplicate_field("end"));
                            }
                            end__ = map_.next_value()?;
                        }
                    }
                }
                Ok(TimeRange {
                    start: start__,
                    end: end__,
                })
            }
        }
        deserializer.deserialize_struct("candela.types.TimeRange", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for ToolAttributes {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.tool_name.is_empty() {
            len += 1;
        }
        if !self.tool_input.is_empty() {
            len += 1;
        }
        if !self.tool_output.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.types.ToolAttributes", len)?;
        if !self.tool_name.is_empty() {
            struct_ser.serialize_field("toolName", &self.tool_name)?;
        }
        if !self.tool_input.is_empty() {
            struct_ser.serialize_field("toolInput", &self.tool_input)?;
        }
        if !self.tool_output.is_empty() {
            struct_ser.serialize_field("toolOutput", &self.tool_output)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for ToolAttributes {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "tool_name",
            "toolName",
            "tool_input",
            "toolInput",
            "tool_output",
            "toolOutput",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            ToolName,
            ToolInput,
            ToolOutput,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "toolName" | "tool_name" => Ok(GeneratedField::ToolName),
                            "toolInput" | "tool_input" => Ok(GeneratedField::ToolInput),
                            "toolOutput" | "tool_output" => Ok(GeneratedField::ToolOutput),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = ToolAttributes;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.types.ToolAttributes")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<ToolAttributes, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut tool_name__ = None;
                let mut tool_input__ = None;
                let mut tool_output__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::ToolName => {
                            if tool_name__.is_some() {
                                return Err(serde::de::Error::duplicate_field("toolName"));
                            }
                            tool_name__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ToolInput => {
                            if tool_input__.is_some() {
                                return Err(serde::de::Error::duplicate_field("toolInput"));
                            }
                            tool_input__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ToolOutput => {
                            if tool_output__.is_some() {
                                return Err(serde::de::Error::duplicate_field("toolOutput"));
                            }
                            tool_output__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(ToolAttributes {
                    tool_name: tool_name__.unwrap_or_default(),
                    tool_input: tool_input__.unwrap_or_default(),
                    tool_output: tool_output__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.types.ToolAttributes", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for Trace {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.trace_id.is_empty() {
            len += 1;
        }
        if self.start_time.is_some() {
            len += 1;
        }
        if self.end_time.is_some() {
            len += 1;
        }
        if self.duration.is_some() {
            len += 1;
        }
        if !self.project_id.is_empty() {
            len += 1;
        }
        if !self.environment.is_empty() {
            len += 1;
        }
        if self.span_count != 0 {
            len += 1;
        }
        if self.total_tokens != 0 {
            len += 1;
        }
        if self.total_cost_usd != 0. {
            len += 1;
        }
        if !self.root_span_name.is_empty() {
            len += 1;
        }
        if !self.user_id.is_empty() {
            len += 1;
        }
        if !self.spans.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.types.Trace", len)?;
        if !self.trace_id.is_empty() {
            struct_ser.serialize_field("traceId", &self.trace_id)?;
        }
        if let Some(v) = self.start_time.as_ref() {
            struct_ser.serialize_field("startTime", v)?;
        }
        if let Some(v) = self.end_time.as_ref() {
            struct_ser.serialize_field("endTime", v)?;
        }
        if let Some(v) = self.duration.as_ref() {
            struct_ser.serialize_field("duration", v)?;
        }
        if !self.project_id.is_empty() {
            struct_ser.serialize_field("projectId", &self.project_id)?;
        }
        if !self.environment.is_empty() {
            struct_ser.serialize_field("environment", &self.environment)?;
        }
        if self.span_count != 0 {
            struct_ser.serialize_field("spanCount", &self.span_count)?;
        }
        if self.total_tokens != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("totalTokens", ToString::to_string(&self.total_tokens).as_str())?;
        }
        if self.total_cost_usd != 0. {
            struct_ser.serialize_field("totalCostUsd", &self.total_cost_usd)?;
        }
        if !self.root_span_name.is_empty() {
            struct_ser.serialize_field("rootSpanName", &self.root_span_name)?;
        }
        if !self.user_id.is_empty() {
            struct_ser.serialize_field("userId", &self.user_id)?;
        }
        if !self.spans.is_empty() {
            struct_ser.serialize_field("spans", &self.spans)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for Trace {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "trace_id",
            "traceId",
            "start_time",
            "startTime",
            "end_time",
            "endTime",
            "duration",
            "project_id",
            "projectId",
            "environment",
            "span_count",
            "spanCount",
            "total_tokens",
            "totalTokens",
            "total_cost_usd",
            "totalCostUsd",
            "root_span_name",
            "rootSpanName",
            "user_id",
            "userId",
            "spans",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            TraceId,
            StartTime,
            EndTime,
            Duration,
            ProjectId,
            Environment,
            SpanCount,
            TotalTokens,
            TotalCostUsd,
            RootSpanName,
            UserId,
            Spans,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "traceId" | "trace_id" => Ok(GeneratedField::TraceId),
                            "startTime" | "start_time" => Ok(GeneratedField::StartTime),
                            "endTime" | "end_time" => Ok(GeneratedField::EndTime),
                            "duration" => Ok(GeneratedField::Duration),
                            "projectId" | "project_id" => Ok(GeneratedField::ProjectId),
                            "environment" => Ok(GeneratedField::Environment),
                            "spanCount" | "span_count" => Ok(GeneratedField::SpanCount),
                            "totalTokens" | "total_tokens" => Ok(GeneratedField::TotalTokens),
                            "totalCostUsd" | "total_cost_usd" => Ok(GeneratedField::TotalCostUsd),
                            "rootSpanName" | "root_span_name" => Ok(GeneratedField::RootSpanName),
                            "userId" | "user_id" => Ok(GeneratedField::UserId),
                            "spans" => Ok(GeneratedField::Spans),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = Trace;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.types.Trace")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<Trace, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut trace_id__ = None;
                let mut start_time__ = None;
                let mut end_time__ = None;
                let mut duration__ = None;
                let mut project_id__ = None;
                let mut environment__ = None;
                let mut span_count__ = None;
                let mut total_tokens__ = None;
                let mut total_cost_usd__ = None;
                let mut root_span_name__ = None;
                let mut user_id__ = None;
                let mut spans__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::TraceId => {
                            if trace_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("traceId"));
                            }
                            trace_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::StartTime => {
                            if start_time__.is_some() {
                                return Err(serde::de::Error::duplicate_field("startTime"));
                            }
                            start_time__ = map_.next_value()?;
                        }
                        GeneratedField::EndTime => {
                            if end_time__.is_some() {
                                return Err(serde::de::Error::duplicate_field("endTime"));
                            }
                            end_time__ = map_.next_value()?;
                        }
                        GeneratedField::Duration => {
                            if duration__.is_some() {
                                return Err(serde::de::Error::duplicate_field("duration"));
                            }
                            duration__ = map_.next_value()?;
                        }
                        GeneratedField::ProjectId => {
                            if project_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("projectId"));
                            }
                            project_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Environment => {
                            if environment__.is_some() {
                                return Err(serde::de::Error::duplicate_field("environment"));
                            }
                            environment__ = Some(map_.next_value()?);
                        }
                        GeneratedField::SpanCount => {
                            if span_count__.is_some() {
                                return Err(serde::de::Error::duplicate_field("spanCount"));
                            }
                            span_count__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::TotalTokens => {
                            if total_tokens__.is_some() {
                                return Err(serde::de::Error::duplicate_field("totalTokens"));
                            }
                            total_tokens__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::TotalCostUsd => {
                            if total_cost_usd__.is_some() {
                                return Err(serde::de::Error::duplicate_field("totalCostUsd"));
                            }
                            total_cost_usd__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::RootSpanName => {
                            if root_span_name__.is_some() {
                                return Err(serde::de::Error::duplicate_field("rootSpanName"));
                            }
                            root_span_name__ = Some(map_.next_value()?);
                        }
                        GeneratedField::UserId => {
                            if user_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("userId"));
                            }
                            user_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Spans => {
                            if spans__.is_some() {
                                return Err(serde::de::Error::duplicate_field("spans"));
                            }
                            spans__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(Trace {
                    trace_id: trace_id__.unwrap_or_default(),
                    start_time: start_time__,
                    end_time: end_time__,
                    duration: duration__,
                    project_id: project_id__.unwrap_or_default(),
                    environment: environment__.unwrap_or_default(),
                    span_count: span_count__.unwrap_or_default(),
                    total_tokens: total_tokens__.unwrap_or_default(),
                    total_cost_usd: total_cost_usd__.unwrap_or_default(),
                    root_span_name: root_span_name__.unwrap_or_default(),
                    user_id: user_id__.unwrap_or_default(),
                    spans: spans__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.types.Trace", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for TraceSummary {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.trace_id.is_empty() {
            len += 1;
        }
        if self.start_time.is_some() {
            len += 1;
        }
        if self.duration.is_some() {
            len += 1;
        }
        if !self.root_span_name.is_empty() {
            len += 1;
        }
        if !self.project_id.is_empty() {
            len += 1;
        }
        if !self.environment.is_empty() {
            len += 1;
        }
        if self.span_count != 0 {
            len += 1;
        }
        if self.llm_call_count != 0 {
            len += 1;
        }
        if self.total_tokens != 0 {
            len += 1;
        }
        if self.total_cost_usd != 0. {
            len += 1;
        }
        if self.status != 0 {
            len += 1;
        }
        if !self.primary_model.is_empty() {
            len += 1;
        }
        if !self.primary_provider.is_empty() {
            len += 1;
        }
        if !self.user_id.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.types.TraceSummary", len)?;
        if !self.trace_id.is_empty() {
            struct_ser.serialize_field("traceId", &self.trace_id)?;
        }
        if let Some(v) = self.start_time.as_ref() {
            struct_ser.serialize_field("startTime", v)?;
        }
        if let Some(v) = self.duration.as_ref() {
            struct_ser.serialize_field("duration", v)?;
        }
        if !self.root_span_name.is_empty() {
            struct_ser.serialize_field("rootSpanName", &self.root_span_name)?;
        }
        if !self.project_id.is_empty() {
            struct_ser.serialize_field("projectId", &self.project_id)?;
        }
        if !self.environment.is_empty() {
            struct_ser.serialize_field("environment", &self.environment)?;
        }
        if self.span_count != 0 {
            struct_ser.serialize_field("spanCount", &self.span_count)?;
        }
        if self.llm_call_count != 0 {
            struct_ser.serialize_field("llmCallCount", &self.llm_call_count)?;
        }
        if self.total_tokens != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("totalTokens", ToString::to_string(&self.total_tokens).as_str())?;
        }
        if self.total_cost_usd != 0. {
            struct_ser.serialize_field("totalCostUsd", &self.total_cost_usd)?;
        }
        if self.status != 0 {
            let v = SpanStatus::try_from(self.status)
                .map_err(|_| serde::ser::Error::custom(format!("Invalid variant {}", self.status)))?;
            struct_ser.serialize_field("status", &v)?;
        }
        if !self.primary_model.is_empty() {
            struct_ser.serialize_field("primaryModel", &self.primary_model)?;
        }
        if !self.primary_provider.is_empty() {
            struct_ser.serialize_field("primaryProvider", &self.primary_provider)?;
        }
        if !self.user_id.is_empty() {
            struct_ser.serialize_field("userId", &self.user_id)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for TraceSummary {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "trace_id",
            "traceId",
            "start_time",
            "startTime",
            "duration",
            "root_span_name",
            "rootSpanName",
            "project_id",
            "projectId",
            "environment",
            "span_count",
            "spanCount",
            "llm_call_count",
            "llmCallCount",
            "total_tokens",
            "totalTokens",
            "total_cost_usd",
            "totalCostUsd",
            "status",
            "primary_model",
            "primaryModel",
            "primary_provider",
            "primaryProvider",
            "user_id",
            "userId",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            TraceId,
            StartTime,
            Duration,
            RootSpanName,
            ProjectId,
            Environment,
            SpanCount,
            LlmCallCount,
            TotalTokens,
            TotalCostUsd,
            Status,
            PrimaryModel,
            PrimaryProvider,
            UserId,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "traceId" | "trace_id" => Ok(GeneratedField::TraceId),
                            "startTime" | "start_time" => Ok(GeneratedField::StartTime),
                            "duration" => Ok(GeneratedField::Duration),
                            "rootSpanName" | "root_span_name" => Ok(GeneratedField::RootSpanName),
                            "projectId" | "project_id" => Ok(GeneratedField::ProjectId),
                            "environment" => Ok(GeneratedField::Environment),
                            "spanCount" | "span_count" => Ok(GeneratedField::SpanCount),
                            "llmCallCount" | "llm_call_count" => Ok(GeneratedField::LlmCallCount),
                            "totalTokens" | "total_tokens" => Ok(GeneratedField::TotalTokens),
                            "totalCostUsd" | "total_cost_usd" => Ok(GeneratedField::TotalCostUsd),
                            "status" => Ok(GeneratedField::Status),
                            "primaryModel" | "primary_model" => Ok(GeneratedField::PrimaryModel),
                            "primaryProvider" | "primary_provider" => Ok(GeneratedField::PrimaryProvider),
                            "userId" | "user_id" => Ok(GeneratedField::UserId),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = TraceSummary;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.types.TraceSummary")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<TraceSummary, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut trace_id__ = None;
                let mut start_time__ = None;
                let mut duration__ = None;
                let mut root_span_name__ = None;
                let mut project_id__ = None;
                let mut environment__ = None;
                let mut span_count__ = None;
                let mut llm_call_count__ = None;
                let mut total_tokens__ = None;
                let mut total_cost_usd__ = None;
                let mut status__ = None;
                let mut primary_model__ = None;
                let mut primary_provider__ = None;
                let mut user_id__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::TraceId => {
                            if trace_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("traceId"));
                            }
                            trace_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::StartTime => {
                            if start_time__.is_some() {
                                return Err(serde::de::Error::duplicate_field("startTime"));
                            }
                            start_time__ = map_.next_value()?;
                        }
                        GeneratedField::Duration => {
                            if duration__.is_some() {
                                return Err(serde::de::Error::duplicate_field("duration"));
                            }
                            duration__ = map_.next_value()?;
                        }
                        GeneratedField::RootSpanName => {
                            if root_span_name__.is_some() {
                                return Err(serde::de::Error::duplicate_field("rootSpanName"));
                            }
                            root_span_name__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ProjectId => {
                            if project_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("projectId"));
                            }
                            project_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Environment => {
                            if environment__.is_some() {
                                return Err(serde::de::Error::duplicate_field("environment"));
                            }
                            environment__ = Some(map_.next_value()?);
                        }
                        GeneratedField::SpanCount => {
                            if span_count__.is_some() {
                                return Err(serde::de::Error::duplicate_field("spanCount"));
                            }
                            span_count__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::LlmCallCount => {
                            if llm_call_count__.is_some() {
                                return Err(serde::de::Error::duplicate_field("llmCallCount"));
                            }
                            llm_call_count__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::TotalTokens => {
                            if total_tokens__.is_some() {
                                return Err(serde::de::Error::duplicate_field("totalTokens"));
                            }
                            total_tokens__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::TotalCostUsd => {
                            if total_cost_usd__.is_some() {
                                return Err(serde::de::Error::duplicate_field("totalCostUsd"));
                            }
                            total_cost_usd__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Status => {
                            if status__.is_some() {
                                return Err(serde::de::Error::duplicate_field("status"));
                            }
                            status__ = Some(map_.next_value::<SpanStatus>()? as i32);
                        }
                        GeneratedField::PrimaryModel => {
                            if primary_model__.is_some() {
                                return Err(serde::de::Error::duplicate_field("primaryModel"));
                            }
                            primary_model__ = Some(map_.next_value()?);
                        }
                        GeneratedField::PrimaryProvider => {
                            if primary_provider__.is_some() {
                                return Err(serde::de::Error::duplicate_field("primaryProvider"));
                            }
                            primary_provider__ = Some(map_.next_value()?);
                        }
                        GeneratedField::UserId => {
                            if user_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("userId"));
                            }
                            user_id__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(TraceSummary {
                    trace_id: trace_id__.unwrap_or_default(),
                    start_time: start_time__,
                    duration: duration__,
                    root_span_name: root_span_name__.unwrap_or_default(),
                    project_id: project_id__.unwrap_or_default(),
                    environment: environment__.unwrap_or_default(),
                    span_count: span_count__.unwrap_or_default(),
                    llm_call_count: llm_call_count__.unwrap_or_default(),
                    total_tokens: total_tokens__.unwrap_or_default(),
                    total_cost_usd: total_cost_usd__.unwrap_or_default(),
                    status: status__.unwrap_or_default(),
                    primary_model: primary_model__.unwrap_or_default(),
                    primary_provider: primary_provider__.unwrap_or_default(),
                    user_id: user_id__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.types.TraceSummary", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for User {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.id.is_empty() {
            len += 1;
        }
        if !self.email.is_empty() {
            len += 1;
        }
        if !self.display_name.is_empty() {
            len += 1;
        }
        if self.role != 0 {
            len += 1;
        }
        if self.status != 0 {
            len += 1;
        }
        if self.created_at.is_some() {
            len += 1;
        }
        if self.last_seen_at.is_some() {
            len += 1;
        }
        if self.rate_limit != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.types.User", len)?;
        if !self.id.is_empty() {
            struct_ser.serialize_field("id", &self.id)?;
        }
        if !self.email.is_empty() {
            struct_ser.serialize_field("email", &self.email)?;
        }
        if !self.display_name.is_empty() {
            struct_ser.serialize_field("displayName", &self.display_name)?;
        }
        if self.role != 0 {
            let v = UserRole::try_from(self.role)
                .map_err(|_| serde::ser::Error::custom(format!("Invalid variant {}", self.role)))?;
            struct_ser.serialize_field("role", &v)?;
        }
        if self.status != 0 {
            let v = UserStatus::try_from(self.status)
                .map_err(|_| serde::ser::Error::custom(format!("Invalid variant {}", self.status)))?;
            struct_ser.serialize_field("status", &v)?;
        }
        if let Some(v) = self.created_at.as_ref() {
            struct_ser.serialize_field("createdAt", v)?;
        }
        if let Some(v) = self.last_seen_at.as_ref() {
            struct_ser.serialize_field("lastSeenAt", v)?;
        }
        if self.rate_limit != 0 {
            struct_ser.serialize_field("rateLimit", &self.rate_limit)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for User {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "id",
            "email",
            "display_name",
            "displayName",
            "role",
            "status",
            "created_at",
            "createdAt",
            "last_seen_at",
            "lastSeenAt",
            "rate_limit",
            "rateLimit",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Id,
            Email,
            DisplayName,
            Role,
            Status,
            CreatedAt,
            LastSeenAt,
            RateLimit,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "id" => Ok(GeneratedField::Id),
                            "email" => Ok(GeneratedField::Email),
                            "displayName" | "display_name" => Ok(GeneratedField::DisplayName),
                            "role" => Ok(GeneratedField::Role),
                            "status" => Ok(GeneratedField::Status),
                            "createdAt" | "created_at" => Ok(GeneratedField::CreatedAt),
                            "lastSeenAt" | "last_seen_at" => Ok(GeneratedField::LastSeenAt),
                            "rateLimit" | "rate_limit" => Ok(GeneratedField::RateLimit),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = User;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.types.User")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<User, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut id__ = None;
                let mut email__ = None;
                let mut display_name__ = None;
                let mut role__ = None;
                let mut status__ = None;
                let mut created_at__ = None;
                let mut last_seen_at__ = None;
                let mut rate_limit__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Id => {
                            if id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("id"));
                            }
                            id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Email => {
                            if email__.is_some() {
                                return Err(serde::de::Error::duplicate_field("email"));
                            }
                            email__ = Some(map_.next_value()?);
                        }
                        GeneratedField::DisplayName => {
                            if display_name__.is_some() {
                                return Err(serde::de::Error::duplicate_field("displayName"));
                            }
                            display_name__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Role => {
                            if role__.is_some() {
                                return Err(serde::de::Error::duplicate_field("role"));
                            }
                            role__ = Some(map_.next_value::<UserRole>()? as i32);
                        }
                        GeneratedField::Status => {
                            if status__.is_some() {
                                return Err(serde::de::Error::duplicate_field("status"));
                            }
                            status__ = Some(map_.next_value::<UserStatus>()? as i32);
                        }
                        GeneratedField::CreatedAt => {
                            if created_at__.is_some() {
                                return Err(serde::de::Error::duplicate_field("createdAt"));
                            }
                            created_at__ = map_.next_value()?;
                        }
                        GeneratedField::LastSeenAt => {
                            if last_seen_at__.is_some() {
                                return Err(serde::de::Error::duplicate_field("lastSeenAt"));
                            }
                            last_seen_at__ = map_.next_value()?;
                        }
                        GeneratedField::RateLimit => {
                            if rate_limit__.is_some() {
                                return Err(serde::de::Error::duplicate_field("rateLimit"));
                            }
                            rate_limit__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(User {
                    id: id__.unwrap_or_default(),
                    email: email__.unwrap_or_default(),
                    display_name: display_name__.unwrap_or_default(),
                    role: role__.unwrap_or_default(),
                    status: status__.unwrap_or_default(),
                    created_at: created_at__,
                    last_seen_at: last_seen_at__,
                    rate_limit: rate_limit__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.types.User", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for UserBudget {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.user_id.is_empty() {
            len += 1;
        }
        if self.limit_usd != 0. {
            len += 1;
        }
        if self.spent_usd != 0. {
            len += 1;
        }
        if self.tokens_used != 0 {
            len += 1;
        }
        if self.period_type != 0 {
            len += 1;
        }
        if !self.period_key.is_empty() {
            len += 1;
        }
        if self.period_start.is_some() {
            len += 1;
        }
        if self.period_end.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.types.UserBudget", len)?;
        if !self.user_id.is_empty() {
            struct_ser.serialize_field("userId", &self.user_id)?;
        }
        if self.limit_usd != 0. {
            struct_ser.serialize_field("limitUsd", &self.limit_usd)?;
        }
        if self.spent_usd != 0. {
            struct_ser.serialize_field("spentUsd", &self.spent_usd)?;
        }
        if self.tokens_used != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("tokensUsed", ToString::to_string(&self.tokens_used).as_str())?;
        }
        if self.period_type != 0 {
            let v = BudgetPeriod::try_from(self.period_type)
                .map_err(|_| serde::ser::Error::custom(format!("Invalid variant {}", self.period_type)))?;
            struct_ser.serialize_field("periodType", &v)?;
        }
        if !self.period_key.is_empty() {
            struct_ser.serialize_field("periodKey", &self.period_key)?;
        }
        if let Some(v) = self.period_start.as_ref() {
            struct_ser.serialize_field("periodStart", v)?;
        }
        if let Some(v) = self.period_end.as_ref() {
            struct_ser.serialize_field("periodEnd", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for UserBudget {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "user_id",
            "userId",
            "limit_usd",
            "limitUsd",
            "spent_usd",
            "spentUsd",
            "tokens_used",
            "tokensUsed",
            "period_type",
            "periodType",
            "period_key",
            "periodKey",
            "period_start",
            "periodStart",
            "period_end",
            "periodEnd",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            UserId,
            LimitUsd,
            SpentUsd,
            TokensUsed,
            PeriodType,
            PeriodKey,
            PeriodStart,
            PeriodEnd,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "userId" | "user_id" => Ok(GeneratedField::UserId),
                            "limitUsd" | "limit_usd" => Ok(GeneratedField::LimitUsd),
                            "spentUsd" | "spent_usd" => Ok(GeneratedField::SpentUsd),
                            "tokensUsed" | "tokens_used" => Ok(GeneratedField::TokensUsed),
                            "periodType" | "period_type" => Ok(GeneratedField::PeriodType),
                            "periodKey" | "period_key" => Ok(GeneratedField::PeriodKey),
                            "periodStart" | "period_start" => Ok(GeneratedField::PeriodStart),
                            "periodEnd" | "period_end" => Ok(GeneratedField::PeriodEnd),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = UserBudget;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.types.UserBudget")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<UserBudget, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut user_id__ = None;
                let mut limit_usd__ = None;
                let mut spent_usd__ = None;
                let mut tokens_used__ = None;
                let mut period_type__ = None;
                let mut period_key__ = None;
                let mut period_start__ = None;
                let mut period_end__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::UserId => {
                            if user_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("userId"));
                            }
                            user_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::LimitUsd => {
                            if limit_usd__.is_some() {
                                return Err(serde::de::Error::duplicate_field("limitUsd"));
                            }
                            limit_usd__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::SpentUsd => {
                            if spent_usd__.is_some() {
                                return Err(serde::de::Error::duplicate_field("spentUsd"));
                            }
                            spent_usd__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::TokensUsed => {
                            if tokens_used__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tokensUsed"));
                            }
                            tokens_used__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::PeriodType => {
                            if period_type__.is_some() {
                                return Err(serde::de::Error::duplicate_field("periodType"));
                            }
                            period_type__ = Some(map_.next_value::<BudgetPeriod>()? as i32);
                        }
                        GeneratedField::PeriodKey => {
                            if period_key__.is_some() {
                                return Err(serde::de::Error::duplicate_field("periodKey"));
                            }
                            period_key__ = Some(map_.next_value()?);
                        }
                        GeneratedField::PeriodStart => {
                            if period_start__.is_some() {
                                return Err(serde::de::Error::duplicate_field("periodStart"));
                            }
                            period_start__ = map_.next_value()?;
                        }
                        GeneratedField::PeriodEnd => {
                            if period_end__.is_some() {
                                return Err(serde::de::Error::duplicate_field("periodEnd"));
                            }
                            period_end__ = map_.next_value()?;
                        }
                    }
                }
                Ok(UserBudget {
                    user_id: user_id__.unwrap_or_default(),
                    limit_usd: limit_usd__.unwrap_or_default(),
                    spent_usd: spent_usd__.unwrap_or_default(),
                    tokens_used: tokens_used__.unwrap_or_default(),
                    period_type: period_type__.unwrap_or_default(),
                    period_key: period_key__.unwrap_or_default(),
                    period_start: period_start__,
                    period_end: period_end__,
                })
            }
        }
        deserializer.deserialize_struct("candela.types.UserBudget", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for UserRole {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        let variant = match self {
            Self::Unspecified => "USER_ROLE_UNSPECIFIED",
            Self::Developer => "USER_ROLE_DEVELOPER",
            Self::Admin => "USER_ROLE_ADMIN",
        };
        serializer.serialize_str(variant)
    }
}
impl<'de> serde::Deserialize<'de> for UserRole {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "USER_ROLE_UNSPECIFIED",
            "USER_ROLE_DEVELOPER",
            "USER_ROLE_ADMIN",
        ];

        struct GeneratedVisitor;

        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = UserRole;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                write!(formatter, "expected one of: {:?}", &FIELDS)
            }

            fn visit_i64<E>(self, v: i64) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                i32::try_from(v)
                    .ok()
                    .and_then(|x| x.try_into().ok())
                    .ok_or_else(|| {
                        serde::de::Error::invalid_value(serde::de::Unexpected::Signed(v), &self)
                    })
            }

            fn visit_u64<E>(self, v: u64) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                i32::try_from(v)
                    .ok()
                    .and_then(|x| x.try_into().ok())
                    .ok_or_else(|| {
                        serde::de::Error::invalid_value(serde::de::Unexpected::Unsigned(v), &self)
                    })
            }

            fn visit_str<E>(self, value: &str) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                match value {
                    "USER_ROLE_UNSPECIFIED" => Ok(UserRole::Unspecified),
                    "USER_ROLE_DEVELOPER" => Ok(UserRole::Developer),
                    "USER_ROLE_ADMIN" => Ok(UserRole::Admin),
                    _ => Err(serde::de::Error::unknown_variant(value, FIELDS)),
                }
            }
        }
        deserializer.deserialize_any(GeneratedVisitor)
    }
}
impl serde::Serialize for UserStatus {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        let variant = match self {
            Self::Unspecified => "USER_STATUS_UNSPECIFIED",
            Self::Provisioned => "USER_STATUS_PROVISIONED",
            Self::Active => "USER_STATUS_ACTIVE",
            Self::Inactive => "USER_STATUS_INACTIVE",
        };
        serializer.serialize_str(variant)
    }
}
impl<'de> serde::Deserialize<'de> for UserStatus {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "USER_STATUS_UNSPECIFIED",
            "USER_STATUS_PROVISIONED",
            "USER_STATUS_ACTIVE",
            "USER_STATUS_INACTIVE",
        ];

        struct GeneratedVisitor;

        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = UserStatus;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                write!(formatter, "expected one of: {:?}", &FIELDS)
            }

            fn visit_i64<E>(self, v: i64) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                i32::try_from(v)
                    .ok()
                    .and_then(|x| x.try_into().ok())
                    .ok_or_else(|| {
                        serde::de::Error::invalid_value(serde::de::Unexpected::Signed(v), &self)
                    })
            }

            fn visit_u64<E>(self, v: u64) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                i32::try_from(v)
                    .ok()
                    .and_then(|x| x.try_into().ok())
                    .ok_or_else(|| {
                        serde::de::Error::invalid_value(serde::de::Unexpected::Unsigned(v), &self)
                    })
            }

            fn visit_str<E>(self, value: &str) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                match value {
                    "USER_STATUS_UNSPECIFIED" => Ok(UserStatus::Unspecified),
                    "USER_STATUS_PROVISIONED" => Ok(UserStatus::Provisioned),
                    "USER_STATUS_ACTIVE" => Ok(UserStatus::Active),
                    "USER_STATUS_INACTIVE" => Ok(UserStatus::Inactive),
                    _ => Err(serde::de::Error::unknown_variant(value, FIELDS)),
                }
            }
        }
        deserializer.deserialize_any(GeneratedVisitor)
    }
}
