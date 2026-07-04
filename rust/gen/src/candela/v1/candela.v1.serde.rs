// @generated
impl serde::Serialize for BackendInfo {
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
        if self.installed {
            len += 1;
        }
        if !self.binary_path.is_empty() {
            len += 1;
        }
        if !self.install_hint.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.BackendInfo", len)?;
        if !self.name.is_empty() {
            struct_ser.serialize_field("name", &self.name)?;
        }
        if self.installed {
            struct_ser.serialize_field("installed", &self.installed)?;
        }
        if !self.binary_path.is_empty() {
            struct_ser.serialize_field("binaryPath", &self.binary_path)?;
        }
        if !self.install_hint.is_empty() {
            struct_ser.serialize_field("installHint", &self.install_hint)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for BackendInfo {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "name",
            "installed",
            "binary_path",
            "binaryPath",
            "install_hint",
            "installHint",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Name,
            Installed,
            BinaryPath,
            InstallHint,
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
                            "installed" => Ok(GeneratedField::Installed),
                            "binaryPath" | "binary_path" => Ok(GeneratedField::BinaryPath),
                            "installHint" | "install_hint" => Ok(GeneratedField::InstallHint),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = BackendInfo;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.BackendInfo")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<BackendInfo, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut name__ = None;
                let mut installed__ = None;
                let mut binary_path__ = None;
                let mut install_hint__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Name => {
                            if name__.is_some() {
                                return Err(serde::de::Error::duplicate_field("name"));
                            }
                            name__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Installed => {
                            if installed__.is_some() {
                                return Err(serde::de::Error::duplicate_field("installed"));
                            }
                            installed__ = Some(map_.next_value()?);
                        }
                        GeneratedField::BinaryPath => {
                            if binary_path__.is_some() {
                                return Err(serde::de::Error::duplicate_field("binaryPath"));
                            }
                            binary_path__ = Some(map_.next_value()?);
                        }
                        GeneratedField::InstallHint => {
                            if install_hint__.is_some() {
                                return Err(serde::de::Error::duplicate_field("installHint"));
                            }
                            install_hint__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(BackendInfo {
                    name: name__.unwrap_or_default(),
                    installed: installed__.unwrap_or_default(),
                    binary_path: binary_path__.unwrap_or_default(),
                    install_hint: install_hint__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.BackendInfo", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for CancelPullRequest {
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
        let mut struct_ser = serializer.serialize_struct("candela.v1.CancelPullRequest", len)?;
        if !self.model.is_empty() {
            struct_ser.serialize_field("model", &self.model)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for CancelPullRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "model",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Model,
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
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = CancelPullRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.CancelPullRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<CancelPullRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut model__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Model => {
                            if model__.is_some() {
                                return Err(serde::de::Error::duplicate_field("model"));
                            }
                            model__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(CancelPullRequest {
                    model: model__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.CancelPullRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for CancelPullResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let len = 0;
        let struct_ser = serializer.serialize_struct("candela.v1.CancelPullResponse", len)?;
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for CancelPullResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
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
                            Err(serde::de::Error::unknown_field(value, FIELDS))
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = CancelPullResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.CancelPullResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<CancelPullResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                while map_.next_key::<GeneratedField>()?.is_some() {
                    let _ = map_.next_value::<serde::de::IgnoredAny>()?;
                }
                Ok(CancelPullResponse {
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.CancelPullResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for CatalogModel {
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
        if !self.size_hint.is_empty() {
            len += 1;
        }
        if self.pinned {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.CatalogModel", len)?;
        if !self.id.is_empty() {
            struct_ser.serialize_field("id", &self.id)?;
        }
        if !self.name.is_empty() {
            struct_ser.serialize_field("name", &self.name)?;
        }
        if !self.description.is_empty() {
            struct_ser.serialize_field("description", &self.description)?;
        }
        if !self.size_hint.is_empty() {
            struct_ser.serialize_field("sizeHint", &self.size_hint)?;
        }
        if self.pinned {
            struct_ser.serialize_field("pinned", &self.pinned)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for CatalogModel {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "id",
            "name",
            "description",
            "size_hint",
            "sizeHint",
            "pinned",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Id,
            Name,
            Description,
            SizeHint,
            Pinned,
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
                            "sizeHint" | "size_hint" => Ok(GeneratedField::SizeHint),
                            "pinned" => Ok(GeneratedField::Pinned),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = CatalogModel;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.CatalogModel")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<CatalogModel, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut id__ = None;
                let mut name__ = None;
                let mut description__ = None;
                let mut size_hint__ = None;
                let mut pinned__ = None;
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
                        GeneratedField::SizeHint => {
                            if size_hint__.is_some() {
                                return Err(serde::de::Error::duplicate_field("sizeHint"));
                            }
                            size_hint__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Pinned => {
                            if pinned__.is_some() {
                                return Err(serde::de::Error::duplicate_field("pinned"));
                            }
                            pinned__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(CatalogModel {
                    id: id__.unwrap_or_default(),
                    name: name__.unwrap_or_default(),
                    description: description__.unwrap_or_default(),
                    size_hint: size_hint__.unwrap_or_default(),
                    pinned: pinned__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.CatalogModel", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for CreateApiKeyRequest {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.project_id.is_empty() {
            len += 1;
        }
        if !self.name.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.CreateAPIKeyRequest", len)?;
        if !self.project_id.is_empty() {
            struct_ser.serialize_field("projectId", &self.project_id)?;
        }
        if !self.name.is_empty() {
            struct_ser.serialize_field("name", &self.name)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for CreateApiKeyRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "project_id",
            "projectId",
            "name",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            ProjectId,
            Name,
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
                            "projectId" | "project_id" => Ok(GeneratedField::ProjectId),
                            "name" => Ok(GeneratedField::Name),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = CreateApiKeyRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.CreateAPIKeyRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<CreateApiKeyRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut project_id__ = None;
                let mut name__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
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
                    }
                }
                Ok(CreateApiKeyRequest {
                    project_id: project_id__.unwrap_or_default(),
                    name: name__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.CreateAPIKeyRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for CreateApiKeyResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.api_key.is_some() {
            len += 1;
        }
        if !self.full_key.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.CreateAPIKeyResponse", len)?;
        if let Some(v) = self.api_key.as_ref() {
            struct_ser.serialize_field("apiKey", v)?;
        }
        if !self.full_key.is_empty() {
            struct_ser.serialize_field("fullKey", &self.full_key)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for CreateApiKeyResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "api_key",
            "apiKey",
            "full_key",
            "fullKey",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            ApiKey,
            FullKey,
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
                            "apiKey" | "api_key" => Ok(GeneratedField::ApiKey),
                            "fullKey" | "full_key" => Ok(GeneratedField::FullKey),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = CreateApiKeyResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.CreateAPIKeyResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<CreateApiKeyResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut api_key__ = None;
                let mut full_key__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::ApiKey => {
                            if api_key__.is_some() {
                                return Err(serde::de::Error::duplicate_field("apiKey"));
                            }
                            api_key__ = map_.next_value()?;
                        }
                        GeneratedField::FullKey => {
                            if full_key__.is_some() {
                                return Err(serde::de::Error::duplicate_field("fullKey"));
                            }
                            full_key__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(CreateApiKeyResponse {
                    api_key: api_key__,
                    full_key: full_key__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.CreateAPIKeyResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for CreateGrantRequest {
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
        if self.amount_usd != 0. {
            len += 1;
        }
        if !self.reason.is_empty() {
            len += 1;
        }
        if self.starts_at.is_some() {
            len += 1;
        }
        if self.expires_at.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.CreateGrantRequest", len)?;
        if !self.user_id.is_empty() {
            struct_ser.serialize_field("userId", &self.user_id)?;
        }
        if self.amount_usd != 0. {
            struct_ser.serialize_field("amountUsd", &self.amount_usd)?;
        }
        if !self.reason.is_empty() {
            struct_ser.serialize_field("reason", &self.reason)?;
        }
        if let Some(v) = self.starts_at.as_ref() {
            struct_ser.serialize_field("startsAt", v)?;
        }
        if let Some(v) = self.expires_at.as_ref() {
            struct_ser.serialize_field("expiresAt", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for CreateGrantRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "user_id",
            "userId",
            "amount_usd",
            "amountUsd",
            "reason",
            "starts_at",
            "startsAt",
            "expires_at",
            "expiresAt",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            UserId,
            AmountUsd,
            Reason,
            StartsAt,
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
                            "userId" | "user_id" => Ok(GeneratedField::UserId),
                            "amountUsd" | "amount_usd" => Ok(GeneratedField::AmountUsd),
                            "reason" => Ok(GeneratedField::Reason),
                            "startsAt" | "starts_at" => Ok(GeneratedField::StartsAt),
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
            type Value = CreateGrantRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.CreateGrantRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<CreateGrantRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut user_id__ = None;
                let mut amount_usd__ = None;
                let mut reason__ = None;
                let mut starts_at__ = None;
                let mut expires_at__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
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
                        GeneratedField::Reason => {
                            if reason__.is_some() {
                                return Err(serde::de::Error::duplicate_field("reason"));
                            }
                            reason__ = Some(map_.next_value()?);
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
                    }
                }
                Ok(CreateGrantRequest {
                    user_id: user_id__.unwrap_or_default(),
                    amount_usd: amount_usd__.unwrap_or_default(),
                    reason: reason__.unwrap_or_default(),
                    starts_at: starts_at__,
                    expires_at: expires_at__,
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.CreateGrantRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for CreateGrantResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.grant.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.CreateGrantResponse", len)?;
        if let Some(v) = self.grant.as_ref() {
            struct_ser.serialize_field("grant", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for CreateGrantResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "grant",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Grant,
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
                            "grant" => Ok(GeneratedField::Grant),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = CreateGrantResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.CreateGrantResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<CreateGrantResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut grant__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Grant => {
                            if grant__.is_some() {
                                return Err(serde::de::Error::duplicate_field("grant"));
                            }
                            grant__ = map_.next_value()?;
                        }
                    }
                }
                Ok(CreateGrantResponse {
                    grant: grant__,
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.CreateGrantResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for CreateProjectRequest {
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
        if !self.description.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.CreateProjectRequest", len)?;
        if !self.name.is_empty() {
            struct_ser.serialize_field("name", &self.name)?;
        }
        if !self.description.is_empty() {
            struct_ser.serialize_field("description", &self.description)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for CreateProjectRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "name",
            "description",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Name,
            Description,
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
                            "description" => Ok(GeneratedField::Description),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = CreateProjectRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.CreateProjectRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<CreateProjectRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut name__ = None;
                let mut description__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
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
                    }
                }
                Ok(CreateProjectRequest {
                    name: name__.unwrap_or_default(),
                    description: description__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.CreateProjectRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for CreateProjectResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.project.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.CreateProjectResponse", len)?;
        if let Some(v) = self.project.as_ref() {
            struct_ser.serialize_field("project", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for CreateProjectResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "project",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Project,
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
                            "project" => Ok(GeneratedField::Project),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = CreateProjectResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.CreateProjectResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<CreateProjectResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut project__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Project => {
                            if project__.is_some() {
                                return Err(serde::de::Error::duplicate_field("project"));
                            }
                            project__ = map_.next_value()?;
                        }
                    }
                }
                Ok(CreateProjectResponse {
                    project: project__,
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.CreateProjectResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for CreateUserRequest {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.email.is_empty() {
            len += 1;
        }
        if !self.display_name.is_empty() {
            len += 1;
        }
        if self.role != 0 {
            len += 1;
        }
        if self.daily_budget_usd != 0. {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.CreateUserRequest", len)?;
        if !self.email.is_empty() {
            struct_ser.serialize_field("email", &self.email)?;
        }
        if !self.display_name.is_empty() {
            struct_ser.serialize_field("displayName", &self.display_name)?;
        }
        if self.role != 0 {
            let v = super::types::UserRole::try_from(self.role)
                .map_err(|_| serde::ser::Error::custom(format!("Invalid variant {}", self.role)))?;
            struct_ser.serialize_field("role", &v)?;
        }
        if self.daily_budget_usd != 0. {
            struct_ser.serialize_field("dailyBudgetUsd", &self.daily_budget_usd)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for CreateUserRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "email",
            "display_name",
            "displayName",
            "role",
            "daily_budget_usd",
            "dailyBudgetUsd",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Email,
            DisplayName,
            Role,
            DailyBudgetUsd,
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
                            "email" => Ok(GeneratedField::Email),
                            "displayName" | "display_name" => Ok(GeneratedField::DisplayName),
                            "role" => Ok(GeneratedField::Role),
                            "dailyBudgetUsd" | "daily_budget_usd" => Ok(GeneratedField::DailyBudgetUsd),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = CreateUserRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.CreateUserRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<CreateUserRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut email__ = None;
                let mut display_name__ = None;
                let mut role__ = None;
                let mut daily_budget_usd__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
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
                            role__ = Some(map_.next_value::<super::types::UserRole>()? as i32);
                        }
                        GeneratedField::DailyBudgetUsd => {
                            if daily_budget_usd__.is_some() {
                                return Err(serde::de::Error::duplicate_field("dailyBudgetUsd"));
                            }
                            daily_budget_usd__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(CreateUserRequest {
                    email: email__.unwrap_or_default(),
                    display_name: display_name__.unwrap_or_default(),
                    role: role__.unwrap_or_default(),
                    daily_budget_usd: daily_budget_usd__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.CreateUserRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for CreateUserResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.user.is_some() {
            len += 1;
        }
        if self.budget.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.CreateUserResponse", len)?;
        if let Some(v) = self.user.as_ref() {
            struct_ser.serialize_field("user", v)?;
        }
        if let Some(v) = self.budget.as_ref() {
            struct_ser.serialize_field("budget", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for CreateUserResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "user",
            "budget",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            User,
            Budget,
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
                            "user" => Ok(GeneratedField::User),
                            "budget" => Ok(GeneratedField::Budget),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = CreateUserResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.CreateUserResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<CreateUserResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut user__ = None;
                let mut budget__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::User => {
                            if user__.is_some() {
                                return Err(serde::de::Error::duplicate_field("user"));
                            }
                            user__ = map_.next_value()?;
                        }
                        GeneratedField::Budget => {
                            if budget__.is_some() {
                                return Err(serde::de::Error::duplicate_field("budget"));
                            }
                            budget__ = map_.next_value()?;
                        }
                    }
                }
                Ok(CreateUserResponse {
                    user: user__,
                    budget: budget__,
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.CreateUserResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for DeactivateUserRequest {
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
        let mut struct_ser = serializer.serialize_struct("candela.v1.DeactivateUserRequest", len)?;
        if !self.id.is_empty() {
            struct_ser.serialize_field("id", &self.id)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for DeactivateUserRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "id",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Id,
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
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = DeactivateUserRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.DeactivateUserRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<DeactivateUserRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut id__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Id => {
                            if id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("id"));
                            }
                            id__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(DeactivateUserRequest {
                    id: id__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.DeactivateUserRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for DeactivateUserResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let len = 0;
        let struct_ser = serializer.serialize_struct("candela.v1.DeactivateUserResponse", len)?;
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for DeactivateUserResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
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
                            Err(serde::de::Error::unknown_field(value, FIELDS))
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = DeactivateUserResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.DeactivateUserResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<DeactivateUserResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                while map_.next_key::<GeneratedField>()?.is_some() {
                    let _ = map_.next_value::<serde::de::IgnoredAny>()?;
                }
                Ok(DeactivateUserResponse {
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.DeactivateUserResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for DeleteModelRequest {
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
        let mut struct_ser = serializer.serialize_struct("candela.v1.DeleteModelRequest", len)?;
        if !self.model.is_empty() {
            struct_ser.serialize_field("model", &self.model)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for DeleteModelRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "model",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Model,
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
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = DeleteModelRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.DeleteModelRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<DeleteModelRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut model__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Model => {
                            if model__.is_some() {
                                return Err(serde::de::Error::duplicate_field("model"));
                            }
                            model__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(DeleteModelRequest {
                    model: model__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.DeleteModelRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for DeleteModelResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let len = 0;
        let struct_ser = serializer.serialize_struct("candela.v1.DeleteModelResponse", len)?;
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for DeleteModelResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
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
                            Err(serde::de::Error::unknown_field(value, FIELDS))
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = DeleteModelResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.DeleteModelResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<DeleteModelResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                while map_.next_key::<GeneratedField>()?.is_some() {
                    let _ = map_.next_value::<serde::de::IgnoredAny>()?;
                }
                Ok(DeleteModelResponse {
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.DeleteModelResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for DeleteProjectRequest {
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
        let mut struct_ser = serializer.serialize_struct("candela.v1.DeleteProjectRequest", len)?;
        if !self.id.is_empty() {
            struct_ser.serialize_field("id", &self.id)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for DeleteProjectRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "id",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Id,
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
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = DeleteProjectRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.DeleteProjectRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<DeleteProjectRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut id__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Id => {
                            if id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("id"));
                            }
                            id__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(DeleteProjectRequest {
                    id: id__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.DeleteProjectRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for DeleteProjectResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let len = 0;
        let struct_ser = serializer.serialize_struct("candela.v1.DeleteProjectResponse", len)?;
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for DeleteProjectResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
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
                            Err(serde::de::Error::unknown_field(value, FIELDS))
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = DeleteProjectResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.DeleteProjectResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<DeleteProjectResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                while map_.next_key::<GeneratedField>()?.is_some() {
                    let _ = map_.next_value::<serde::de::IgnoredAny>()?;
                }
                Ok(DeleteProjectResponse {
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.DeleteProjectResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for DeleteUserRequest {
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
        if !self.confirm_email.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.DeleteUserRequest", len)?;
        if !self.id.is_empty() {
            struct_ser.serialize_field("id", &self.id)?;
        }
        if !self.confirm_email.is_empty() {
            struct_ser.serialize_field("confirmEmail", &self.confirm_email)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for DeleteUserRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "id",
            "confirm_email",
            "confirmEmail",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Id,
            ConfirmEmail,
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
                            "confirmEmail" | "confirm_email" => Ok(GeneratedField::ConfirmEmail),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = DeleteUserRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.DeleteUserRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<DeleteUserRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut id__ = None;
                let mut confirm_email__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Id => {
                            if id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("id"));
                            }
                            id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ConfirmEmail => {
                            if confirm_email__.is_some() {
                                return Err(serde::de::Error::duplicate_field("confirmEmail"));
                            }
                            confirm_email__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(DeleteUserRequest {
                    id: id__.unwrap_or_default(),
                    confirm_email: confirm_email__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.DeleteUserRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for DeleteUserResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let len = 0;
        let struct_ser = serializer.serialize_struct("candela.v1.DeleteUserResponse", len)?;
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for DeleteUserResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
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
                            Err(serde::de::Error::unknown_field(value, FIELDS))
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = DeleteUserResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.DeleteUserResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<DeleteUserResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                while map_.next_key::<GeneratedField>()?.is_some() {
                    let _ = map_.next_value::<serde::de::IgnoredAny>()?;
                }
                Ok(DeleteUserResponse {
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.DeleteUserResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for GetBudgetRequest {
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
        let mut struct_ser = serializer.serialize_struct("candela.v1.GetBudgetRequest", len)?;
        if !self.user_id.is_empty() {
            struct_ser.serialize_field("userId", &self.user_id)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for GetBudgetRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "user_id",
            "userId",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
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
            type Value = GetBudgetRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.GetBudgetRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<GetBudgetRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut user_id__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::UserId => {
                            if user_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("userId"));
                            }
                            user_id__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(GetBudgetRequest {
                    user_id: user_id__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.GetBudgetRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for GetBudgetResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.budget.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.GetBudgetResponse", len)?;
        if let Some(v) = self.budget.as_ref() {
            struct_ser.serialize_field("budget", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for GetBudgetResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "budget",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Budget,
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
                            "budget" => Ok(GeneratedField::Budget),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = GetBudgetResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.GetBudgetResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<GetBudgetResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut budget__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Budget => {
                            if budget__.is_some() {
                                return Err(serde::de::Error::duplicate_field("budget"));
                            }
                            budget__ = map_.next_value()?;
                        }
                    }
                }
                Ok(GetBudgetResponse {
                    budget: budget__,
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.GetBudgetResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for GetCurrentUserRequest {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let len = 0;
        let struct_ser = serializer.serialize_struct("candela.v1.GetCurrentUserRequest", len)?;
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for GetCurrentUserRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
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
                            Err(serde::de::Error::unknown_field(value, FIELDS))
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = GetCurrentUserRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.GetCurrentUserRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<GetCurrentUserRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                while map_.next_key::<GeneratedField>()?.is_some() {
                    let _ = map_.next_value::<serde::de::IgnoredAny>()?;
                }
                Ok(GetCurrentUserRequest {
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.GetCurrentUserRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for GetCurrentUserResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.user.is_some() {
            len += 1;
        }
        if self.budget.is_some() {
            len += 1;
        }
        if !self.active_grants.is_empty() {
            len += 1;
        }
        if self.total_remaining_usd != 0. {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.GetCurrentUserResponse", len)?;
        if let Some(v) = self.user.as_ref() {
            struct_ser.serialize_field("user", v)?;
        }
        if let Some(v) = self.budget.as_ref() {
            struct_ser.serialize_field("budget", v)?;
        }
        if !self.active_grants.is_empty() {
            struct_ser.serialize_field("activeGrants", &self.active_grants)?;
        }
        if self.total_remaining_usd != 0. {
            struct_ser.serialize_field("totalRemainingUsd", &self.total_remaining_usd)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for GetCurrentUserResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "user",
            "budget",
            "active_grants",
            "activeGrants",
            "total_remaining_usd",
            "totalRemainingUsd",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            User,
            Budget,
            ActiveGrants,
            TotalRemainingUsd,
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
                            "user" => Ok(GeneratedField::User),
                            "budget" => Ok(GeneratedField::Budget),
                            "activeGrants" | "active_grants" => Ok(GeneratedField::ActiveGrants),
                            "totalRemainingUsd" | "total_remaining_usd" => Ok(GeneratedField::TotalRemainingUsd),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = GetCurrentUserResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.GetCurrentUserResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<GetCurrentUserResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut user__ = None;
                let mut budget__ = None;
                let mut active_grants__ = None;
                let mut total_remaining_usd__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::User => {
                            if user__.is_some() {
                                return Err(serde::de::Error::duplicate_field("user"));
                            }
                            user__ = map_.next_value()?;
                        }
                        GeneratedField::Budget => {
                            if budget__.is_some() {
                                return Err(serde::de::Error::duplicate_field("budget"));
                            }
                            budget__ = map_.next_value()?;
                        }
                        GeneratedField::ActiveGrants => {
                            if active_grants__.is_some() {
                                return Err(serde::de::Error::duplicate_field("activeGrants"));
                            }
                            active_grants__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TotalRemainingUsd => {
                            if total_remaining_usd__.is_some() {
                                return Err(serde::de::Error::duplicate_field("totalRemainingUsd"));
                            }
                            total_remaining_usd__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(GetCurrentUserResponse {
                    user: user__,
                    budget: budget__,
                    active_grants: active_grants__.unwrap_or_default(),
                    total_remaining_usd: total_remaining_usd__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.GetCurrentUserResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for GetHealthRequest {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let len = 0;
        let struct_ser = serializer.serialize_struct("candela.v1.GetHealthRequest", len)?;
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for GetHealthRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
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
                            Err(serde::de::Error::unknown_field(value, FIELDS))
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = GetHealthRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.GetHealthRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<GetHealthRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                while map_.next_key::<GeneratedField>()?.is_some() {
                    let _ = map_.next_value::<serde::de::IgnoredAny>()?;
                }
                Ok(GetHealthRequest {
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.GetHealthRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for GetHealthResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.status.is_some() {
            len += 1;
        }
        if !self.models.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.GetHealthResponse", len)?;
        if let Some(v) = self.status.as_ref() {
            struct_ser.serialize_field("status", v)?;
        }
        if !self.models.is_empty() {
            struct_ser.serialize_field("models", &self.models)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for GetHealthResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "status",
            "models",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Status,
            Models,
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
                            "status" => Ok(GeneratedField::Status),
                            "models" => Ok(GeneratedField::Models),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = GetHealthResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.GetHealthResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<GetHealthResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut status__ = None;
                let mut models__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Status => {
                            if status__.is_some() {
                                return Err(serde::de::Error::duplicate_field("status"));
                            }
                            status__ = map_.next_value()?;
                        }
                        GeneratedField::Models => {
                            if models__.is_some() {
                                return Err(serde::de::Error::duplicate_field("models"));
                            }
                            models__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(GetHealthResponse {
                    status: status__,
                    models: models__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.GetHealthResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for GetLatencyPercentilesRequest {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.project_id.is_empty() {
            len += 1;
        }
        if self.time_range.is_some() {
            len += 1;
        }
        if !self.model.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.GetLatencyPercentilesRequest", len)?;
        if !self.project_id.is_empty() {
            struct_ser.serialize_field("projectId", &self.project_id)?;
        }
        if let Some(v) = self.time_range.as_ref() {
            struct_ser.serialize_field("timeRange", v)?;
        }
        if !self.model.is_empty() {
            struct_ser.serialize_field("model", &self.model)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for GetLatencyPercentilesRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "project_id",
            "projectId",
            "time_range",
            "timeRange",
            "model",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            ProjectId,
            TimeRange,
            Model,
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
                            "projectId" | "project_id" => Ok(GeneratedField::ProjectId),
                            "timeRange" | "time_range" => Ok(GeneratedField::TimeRange),
                            "model" => Ok(GeneratedField::Model),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = GetLatencyPercentilesRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.GetLatencyPercentilesRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<GetLatencyPercentilesRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut project_id__ = None;
                let mut time_range__ = None;
                let mut model__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::ProjectId => {
                            if project_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("projectId"));
                            }
                            project_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TimeRange => {
                            if time_range__.is_some() {
                                return Err(serde::de::Error::duplicate_field("timeRange"));
                            }
                            time_range__ = map_.next_value()?;
                        }
                        GeneratedField::Model => {
                            if model__.is_some() {
                                return Err(serde::de::Error::duplicate_field("model"));
                            }
                            model__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(GetLatencyPercentilesRequest {
                    project_id: project_id__.unwrap_or_default(),
                    time_range: time_range__,
                    model: model__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.GetLatencyPercentilesRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for GetLatencyPercentilesResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.p50_ms != 0. {
            len += 1;
        }
        if self.p90_ms != 0. {
            len += 1;
        }
        if self.p95_ms != 0. {
            len += 1;
        }
        if self.p99_ms != 0. {
            len += 1;
        }
        if !self.latency_over_time.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.GetLatencyPercentilesResponse", len)?;
        if self.p50_ms != 0. {
            struct_ser.serialize_field("p50Ms", &self.p50_ms)?;
        }
        if self.p90_ms != 0. {
            struct_ser.serialize_field("p90Ms", &self.p90_ms)?;
        }
        if self.p95_ms != 0. {
            struct_ser.serialize_field("p95Ms", &self.p95_ms)?;
        }
        if self.p99_ms != 0. {
            struct_ser.serialize_field("p99Ms", &self.p99_ms)?;
        }
        if !self.latency_over_time.is_empty() {
            struct_ser.serialize_field("latencyOverTime", &self.latency_over_time)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for GetLatencyPercentilesResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "p50_ms",
            "p50Ms",
            "p90_ms",
            "p90Ms",
            "p95_ms",
            "p95Ms",
            "p99_ms",
            "p99Ms",
            "latency_over_time",
            "latencyOverTime",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            P50Ms,
            P90Ms,
            P95Ms,
            P99Ms,
            LatencyOverTime,
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
                            "p50Ms" | "p50_ms" => Ok(GeneratedField::P50Ms),
                            "p90Ms" | "p90_ms" => Ok(GeneratedField::P90Ms),
                            "p95Ms" | "p95_ms" => Ok(GeneratedField::P95Ms),
                            "p99Ms" | "p99_ms" => Ok(GeneratedField::P99Ms),
                            "latencyOverTime" | "latency_over_time" => Ok(GeneratedField::LatencyOverTime),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = GetLatencyPercentilesResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.GetLatencyPercentilesResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<GetLatencyPercentilesResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut p50_ms__ = None;
                let mut p90_ms__ = None;
                let mut p95_ms__ = None;
                let mut p99_ms__ = None;
                let mut latency_over_time__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::P50Ms => {
                            if p50_ms__.is_some() {
                                return Err(serde::de::Error::duplicate_field("p50Ms"));
                            }
                            p50_ms__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::P90Ms => {
                            if p90_ms__.is_some() {
                                return Err(serde::de::Error::duplicate_field("p90Ms"));
                            }
                            p90_ms__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::P95Ms => {
                            if p95_ms__.is_some() {
                                return Err(serde::de::Error::duplicate_field("p95Ms"));
                            }
                            p95_ms__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::P99Ms => {
                            if p99_ms__.is_some() {
                                return Err(serde::de::Error::duplicate_field("p99Ms"));
                            }
                            p99_ms__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::LatencyOverTime => {
                            if latency_over_time__.is_some() {
                                return Err(serde::de::Error::duplicate_field("latencyOverTime"));
                            }
                            latency_over_time__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(GetLatencyPercentilesResponse {
                    p50_ms: p50_ms__.unwrap_or_default(),
                    p90_ms: p90_ms__.unwrap_or_default(),
                    p95_ms: p95_ms__.unwrap_or_default(),
                    p99_ms: p99_ms__.unwrap_or_default(),
                    latency_over_time: latency_over_time__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.GetLatencyPercentilesResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for GetModelBreakdownRequest {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.project_id.is_empty() {
            len += 1;
        }
        if self.time_range.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.GetModelBreakdownRequest", len)?;
        if !self.project_id.is_empty() {
            struct_ser.serialize_field("projectId", &self.project_id)?;
        }
        if let Some(v) = self.time_range.as_ref() {
            struct_ser.serialize_field("timeRange", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for GetModelBreakdownRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "project_id",
            "projectId",
            "time_range",
            "timeRange",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            ProjectId,
            TimeRange,
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
                            "projectId" | "project_id" => Ok(GeneratedField::ProjectId),
                            "timeRange" | "time_range" => Ok(GeneratedField::TimeRange),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = GetModelBreakdownRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.GetModelBreakdownRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<GetModelBreakdownRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut project_id__ = None;
                let mut time_range__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::ProjectId => {
                            if project_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("projectId"));
                            }
                            project_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TimeRange => {
                            if time_range__.is_some() {
                                return Err(serde::de::Error::duplicate_field("timeRange"));
                            }
                            time_range__ = map_.next_value()?;
                        }
                    }
                }
                Ok(GetModelBreakdownRequest {
                    project_id: project_id__.unwrap_or_default(),
                    time_range: time_range__,
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.GetModelBreakdownRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for GetModelBreakdownResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.models.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.GetModelBreakdownResponse", len)?;
        if !self.models.is_empty() {
            struct_ser.serialize_field("models", &self.models)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for GetModelBreakdownResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "models",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Models,
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
                            "models" => Ok(GeneratedField::Models),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = GetModelBreakdownResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.GetModelBreakdownResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<GetModelBreakdownResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut models__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Models => {
                            if models__.is_some() {
                                return Err(serde::de::Error::duplicate_field("models"));
                            }
                            models__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(GetModelBreakdownResponse {
                    models: models__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.GetModelBreakdownResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for GetMyBudgetRequest {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let len = 0;
        let struct_ser = serializer.serialize_struct("candela.v1.GetMyBudgetRequest", len)?;
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for GetMyBudgetRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
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
                            Err(serde::de::Error::unknown_field(value, FIELDS))
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = GetMyBudgetRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.GetMyBudgetRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<GetMyBudgetRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                while map_.next_key::<GeneratedField>()?.is_some() {
                    let _ = map_.next_value::<serde::de::IgnoredAny>()?;
                }
                Ok(GetMyBudgetRequest {
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.GetMyBudgetRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for GetMyBudgetResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.budget.is_some() {
            len += 1;
        }
        if !self.active_grants.is_empty() {
            len += 1;
        }
        if self.total_remaining_usd != 0. {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.GetMyBudgetResponse", len)?;
        if let Some(v) = self.budget.as_ref() {
            struct_ser.serialize_field("budget", v)?;
        }
        if !self.active_grants.is_empty() {
            struct_ser.serialize_field("activeGrants", &self.active_grants)?;
        }
        if self.total_remaining_usd != 0. {
            struct_ser.serialize_field("totalRemainingUsd", &self.total_remaining_usd)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for GetMyBudgetResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "budget",
            "active_grants",
            "activeGrants",
            "total_remaining_usd",
            "totalRemainingUsd",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Budget,
            ActiveGrants,
            TotalRemainingUsd,
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
                            "budget" => Ok(GeneratedField::Budget),
                            "activeGrants" | "active_grants" => Ok(GeneratedField::ActiveGrants),
                            "totalRemainingUsd" | "total_remaining_usd" => Ok(GeneratedField::TotalRemainingUsd),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = GetMyBudgetResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.GetMyBudgetResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<GetMyBudgetResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut budget__ = None;
                let mut active_grants__ = None;
                let mut total_remaining_usd__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Budget => {
                            if budget__.is_some() {
                                return Err(serde::de::Error::duplicate_field("budget"));
                            }
                            budget__ = map_.next_value()?;
                        }
                        GeneratedField::ActiveGrants => {
                            if active_grants__.is_some() {
                                return Err(serde::de::Error::duplicate_field("activeGrants"));
                            }
                            active_grants__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TotalRemainingUsd => {
                            if total_remaining_usd__.is_some() {
                                return Err(serde::de::Error::duplicate_field("totalRemainingUsd"));
                            }
                            total_remaining_usd__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(GetMyBudgetResponse {
                    budget: budget__,
                    active_grants: active_grants__.unwrap_or_default(),
                    total_remaining_usd: total_remaining_usd__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.GetMyBudgetResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for GetMyUsageRequest {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.project_id.is_empty() {
            len += 1;
        }
        if self.time_range.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.GetMyUsageRequest", len)?;
        if !self.project_id.is_empty() {
            struct_ser.serialize_field("projectId", &self.project_id)?;
        }
        if let Some(v) = self.time_range.as_ref() {
            struct_ser.serialize_field("timeRange", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for GetMyUsageRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "project_id",
            "projectId",
            "time_range",
            "timeRange",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            ProjectId,
            TimeRange,
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
                            "projectId" | "project_id" => Ok(GeneratedField::ProjectId),
                            "timeRange" | "time_range" => Ok(GeneratedField::TimeRange),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = GetMyUsageRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.GetMyUsageRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<GetMyUsageRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut project_id__ = None;
                let mut time_range__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::ProjectId => {
                            if project_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("projectId"));
                            }
                            project_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TimeRange => {
                            if time_range__.is_some() {
                                return Err(serde::de::Error::duplicate_field("timeRange"));
                            }
                            time_range__ = map_.next_value()?;
                        }
                    }
                }
                Ok(GetMyUsageRequest {
                    project_id: project_id__.unwrap_or_default(),
                    time_range: time_range__,
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.GetMyUsageRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for GetMyUsageResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.total_calls != 0 {
            len += 1;
        }
        if self.total_input_tokens != 0 {
            len += 1;
        }
        if self.total_output_tokens != 0 {
            len += 1;
        }
        if self.total_cost_usd != 0. {
            len += 1;
        }
        if self.avg_latency_ms != 0. {
            len += 1;
        }
        if !self.models.is_empty() {
            len += 1;
        }
        if self.budget.is_some() {
            len += 1;
        }
        if self.total_remaining_usd != 0. {
            len += 1;
        }
        if !self.active_grants.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.GetMyUsageResponse", len)?;
        if self.total_calls != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("totalCalls", ToString::to_string(&self.total_calls).as_str())?;
        }
        if self.total_input_tokens != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("totalInputTokens", ToString::to_string(&self.total_input_tokens).as_str())?;
        }
        if self.total_output_tokens != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("totalOutputTokens", ToString::to_string(&self.total_output_tokens).as_str())?;
        }
        if self.total_cost_usd != 0. {
            struct_ser.serialize_field("totalCostUsd", &self.total_cost_usd)?;
        }
        if self.avg_latency_ms != 0. {
            struct_ser.serialize_field("avgLatencyMs", &self.avg_latency_ms)?;
        }
        if !self.models.is_empty() {
            struct_ser.serialize_field("models", &self.models)?;
        }
        if let Some(v) = self.budget.as_ref() {
            struct_ser.serialize_field("budget", v)?;
        }
        if self.total_remaining_usd != 0. {
            struct_ser.serialize_field("totalRemainingUsd", &self.total_remaining_usd)?;
        }
        if !self.active_grants.is_empty() {
            struct_ser.serialize_field("activeGrants", &self.active_grants)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for GetMyUsageResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "total_calls",
            "totalCalls",
            "total_input_tokens",
            "totalInputTokens",
            "total_output_tokens",
            "totalOutputTokens",
            "total_cost_usd",
            "totalCostUsd",
            "avg_latency_ms",
            "avgLatencyMs",
            "models",
            "budget",
            "total_remaining_usd",
            "totalRemainingUsd",
            "active_grants",
            "activeGrants",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            TotalCalls,
            TotalInputTokens,
            TotalOutputTokens,
            TotalCostUsd,
            AvgLatencyMs,
            Models,
            Budget,
            TotalRemainingUsd,
            ActiveGrants,
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
                            "totalCalls" | "total_calls" => Ok(GeneratedField::TotalCalls),
                            "totalInputTokens" | "total_input_tokens" => Ok(GeneratedField::TotalInputTokens),
                            "totalOutputTokens" | "total_output_tokens" => Ok(GeneratedField::TotalOutputTokens),
                            "totalCostUsd" | "total_cost_usd" => Ok(GeneratedField::TotalCostUsd),
                            "avgLatencyMs" | "avg_latency_ms" => Ok(GeneratedField::AvgLatencyMs),
                            "models" => Ok(GeneratedField::Models),
                            "budget" => Ok(GeneratedField::Budget),
                            "totalRemainingUsd" | "total_remaining_usd" => Ok(GeneratedField::TotalRemainingUsd),
                            "activeGrants" | "active_grants" => Ok(GeneratedField::ActiveGrants),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = GetMyUsageResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.GetMyUsageResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<GetMyUsageResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut total_calls__ = None;
                let mut total_input_tokens__ = None;
                let mut total_output_tokens__ = None;
                let mut total_cost_usd__ = None;
                let mut avg_latency_ms__ = None;
                let mut models__ = None;
                let mut budget__ = None;
                let mut total_remaining_usd__ = None;
                let mut active_grants__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::TotalCalls => {
                            if total_calls__.is_some() {
                                return Err(serde::de::Error::duplicate_field("totalCalls"));
                            }
                            total_calls__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::TotalInputTokens => {
                            if total_input_tokens__.is_some() {
                                return Err(serde::de::Error::duplicate_field("totalInputTokens"));
                            }
                            total_input_tokens__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::TotalOutputTokens => {
                            if total_output_tokens__.is_some() {
                                return Err(serde::de::Error::duplicate_field("totalOutputTokens"));
                            }
                            total_output_tokens__ =
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
                        GeneratedField::AvgLatencyMs => {
                            if avg_latency_ms__.is_some() {
                                return Err(serde::de::Error::duplicate_field("avgLatencyMs"));
                            }
                            avg_latency_ms__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Models => {
                            if models__.is_some() {
                                return Err(serde::de::Error::duplicate_field("models"));
                            }
                            models__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Budget => {
                            if budget__.is_some() {
                                return Err(serde::de::Error::duplicate_field("budget"));
                            }
                            budget__ = map_.next_value()?;
                        }
                        GeneratedField::TotalRemainingUsd => {
                            if total_remaining_usd__.is_some() {
                                return Err(serde::de::Error::duplicate_field("totalRemainingUsd"));
                            }
                            total_remaining_usd__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::ActiveGrants => {
                            if active_grants__.is_some() {
                                return Err(serde::de::Error::duplicate_field("activeGrants"));
                            }
                            active_grants__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(GetMyUsageResponse {
                    total_calls: total_calls__.unwrap_or_default(),
                    total_input_tokens: total_input_tokens__.unwrap_or_default(),
                    total_output_tokens: total_output_tokens__.unwrap_or_default(),
                    total_cost_usd: total_cost_usd__.unwrap_or_default(),
                    avg_latency_ms: avg_latency_ms__.unwrap_or_default(),
                    models: models__.unwrap_or_default(),
                    budget: budget__,
                    total_remaining_usd: total_remaining_usd__.unwrap_or_default(),
                    active_grants: active_grants__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.GetMyUsageResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for GetProjectRequest {
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
        let mut struct_ser = serializer.serialize_struct("candela.v1.GetProjectRequest", len)?;
        if !self.id.is_empty() {
            struct_ser.serialize_field("id", &self.id)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for GetProjectRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "id",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Id,
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
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = GetProjectRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.GetProjectRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<GetProjectRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut id__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Id => {
                            if id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("id"));
                            }
                            id__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(GetProjectRequest {
                    id: id__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.GetProjectRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for GetProjectResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.project.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.GetProjectResponse", len)?;
        if let Some(v) = self.project.as_ref() {
            struct_ser.serialize_field("project", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for GetProjectResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "project",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Project,
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
                            "project" => Ok(GeneratedField::Project),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = GetProjectResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.GetProjectResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<GetProjectResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut project__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Project => {
                            if project__.is_some() {
                                return Err(serde::de::Error::duplicate_field("project"));
                            }
                            project__ = map_.next_value()?;
                        }
                    }
                }
                Ok(GetProjectResponse {
                    project: project__,
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.GetProjectResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for GetTeamLeaderboardRequest {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.project_id.is_empty() {
            len += 1;
        }
        if self.time_range.is_some() {
            len += 1;
        }
        if self.limit != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.GetTeamLeaderboardRequest", len)?;
        if !self.project_id.is_empty() {
            struct_ser.serialize_field("projectId", &self.project_id)?;
        }
        if let Some(v) = self.time_range.as_ref() {
            struct_ser.serialize_field("timeRange", v)?;
        }
        if self.limit != 0 {
            struct_ser.serialize_field("limit", &self.limit)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for GetTeamLeaderboardRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "project_id",
            "projectId",
            "time_range",
            "timeRange",
            "limit",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            ProjectId,
            TimeRange,
            Limit,
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
                            "projectId" | "project_id" => Ok(GeneratedField::ProjectId),
                            "timeRange" | "time_range" => Ok(GeneratedField::TimeRange),
                            "limit" => Ok(GeneratedField::Limit),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = GetTeamLeaderboardRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.GetTeamLeaderboardRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<GetTeamLeaderboardRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut project_id__ = None;
                let mut time_range__ = None;
                let mut limit__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::ProjectId => {
                            if project_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("projectId"));
                            }
                            project_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TimeRange => {
                            if time_range__.is_some() {
                                return Err(serde::de::Error::duplicate_field("timeRange"));
                            }
                            time_range__ = map_.next_value()?;
                        }
                        GeneratedField::Limit => {
                            if limit__.is_some() {
                                return Err(serde::de::Error::duplicate_field("limit"));
                            }
                            limit__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(GetTeamLeaderboardRequest {
                    project_id: project_id__.unwrap_or_default(),
                    time_range: time_range__,
                    limit: limit__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.GetTeamLeaderboardRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for GetTeamLeaderboardResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.users.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.GetTeamLeaderboardResponse", len)?;
        if !self.users.is_empty() {
            struct_ser.serialize_field("users", &self.users)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for GetTeamLeaderboardResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "users",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Users,
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
                            "users" => Ok(GeneratedField::Users),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = GetTeamLeaderboardResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.GetTeamLeaderboardResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<GetTeamLeaderboardResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut users__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Users => {
                            if users__.is_some() {
                                return Err(serde::de::Error::duplicate_field("users"));
                            }
                            users__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(GetTeamLeaderboardResponse {
                    users: users__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.GetTeamLeaderboardResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for GetTraceRequest {
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
        let mut struct_ser = serializer.serialize_struct("candela.v1.GetTraceRequest", len)?;
        if !self.trace_id.is_empty() {
            struct_ser.serialize_field("traceId", &self.trace_id)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for GetTraceRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "trace_id",
            "traceId",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            TraceId,
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
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = GetTraceRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.GetTraceRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<GetTraceRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut trace_id__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::TraceId => {
                            if trace_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("traceId"));
                            }
                            trace_id__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(GetTraceRequest {
                    trace_id: trace_id__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.GetTraceRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for GetTraceResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.trace.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.GetTraceResponse", len)?;
        if let Some(v) = self.trace.as_ref() {
            struct_ser.serialize_field("trace", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for GetTraceResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "trace",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Trace,
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
                            "trace" => Ok(GeneratedField::Trace),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = GetTraceResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.GetTraceResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<GetTraceResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut trace__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Trace => {
                            if trace__.is_some() {
                                return Err(serde::de::Error::duplicate_field("trace"));
                            }
                            trace__ = map_.next_value()?;
                        }
                    }
                }
                Ok(GetTraceResponse {
                    trace: trace__,
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.GetTraceResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for GetUsageSummaryRequest {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.project_id.is_empty() {
            len += 1;
        }
        if self.time_range.is_some() {
            len += 1;
        }
        if !self.environment.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.GetUsageSummaryRequest", len)?;
        if !self.project_id.is_empty() {
            struct_ser.serialize_field("projectId", &self.project_id)?;
        }
        if let Some(v) = self.time_range.as_ref() {
            struct_ser.serialize_field("timeRange", v)?;
        }
        if !self.environment.is_empty() {
            struct_ser.serialize_field("environment", &self.environment)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for GetUsageSummaryRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "project_id",
            "projectId",
            "time_range",
            "timeRange",
            "environment",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            ProjectId,
            TimeRange,
            Environment,
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
                            "projectId" | "project_id" => Ok(GeneratedField::ProjectId),
                            "timeRange" | "time_range" => Ok(GeneratedField::TimeRange),
                            "environment" => Ok(GeneratedField::Environment),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = GetUsageSummaryRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.GetUsageSummaryRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<GetUsageSummaryRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut project_id__ = None;
                let mut time_range__ = None;
                let mut environment__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::ProjectId => {
                            if project_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("projectId"));
                            }
                            project_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TimeRange => {
                            if time_range__.is_some() {
                                return Err(serde::de::Error::duplicate_field("timeRange"));
                            }
                            time_range__ = map_.next_value()?;
                        }
                        GeneratedField::Environment => {
                            if environment__.is_some() {
                                return Err(serde::de::Error::duplicate_field("environment"));
                            }
                            environment__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(GetUsageSummaryRequest {
                    project_id: project_id__.unwrap_or_default(),
                    time_range: time_range__,
                    environment: environment__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.GetUsageSummaryRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for GetUsageSummaryResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.total_traces != 0 {
            len += 1;
        }
        if self.total_spans != 0 {
            len += 1;
        }
        if self.total_llm_calls != 0 {
            len += 1;
        }
        if self.total_input_tokens != 0 {
            len += 1;
        }
        if self.total_output_tokens != 0 {
            len += 1;
        }
        if self.total_cost_usd != 0. {
            len += 1;
        }
        if self.avg_latency_ms != 0. {
            len += 1;
        }
        if self.error_rate != 0. {
            len += 1;
        }
        if !self.traces_over_time.is_empty() {
            len += 1;
        }
        if !self.cost_over_time.is_empty() {
            len += 1;
        }
        if !self.tokens_over_time.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.GetUsageSummaryResponse", len)?;
        if self.total_traces != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("totalTraces", ToString::to_string(&self.total_traces).as_str())?;
        }
        if self.total_spans != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("totalSpans", ToString::to_string(&self.total_spans).as_str())?;
        }
        if self.total_llm_calls != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("totalLlmCalls", ToString::to_string(&self.total_llm_calls).as_str())?;
        }
        if self.total_input_tokens != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("totalInputTokens", ToString::to_string(&self.total_input_tokens).as_str())?;
        }
        if self.total_output_tokens != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("totalOutputTokens", ToString::to_string(&self.total_output_tokens).as_str())?;
        }
        if self.total_cost_usd != 0. {
            struct_ser.serialize_field("totalCostUsd", &self.total_cost_usd)?;
        }
        if self.avg_latency_ms != 0. {
            struct_ser.serialize_field("avgLatencyMs", &self.avg_latency_ms)?;
        }
        if self.error_rate != 0. {
            struct_ser.serialize_field("errorRate", &self.error_rate)?;
        }
        if !self.traces_over_time.is_empty() {
            struct_ser.serialize_field("tracesOverTime", &self.traces_over_time)?;
        }
        if !self.cost_over_time.is_empty() {
            struct_ser.serialize_field("costOverTime", &self.cost_over_time)?;
        }
        if !self.tokens_over_time.is_empty() {
            struct_ser.serialize_field("tokensOverTime", &self.tokens_over_time)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for GetUsageSummaryResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "total_traces",
            "totalTraces",
            "total_spans",
            "totalSpans",
            "total_llm_calls",
            "totalLlmCalls",
            "total_input_tokens",
            "totalInputTokens",
            "total_output_tokens",
            "totalOutputTokens",
            "total_cost_usd",
            "totalCostUsd",
            "avg_latency_ms",
            "avgLatencyMs",
            "error_rate",
            "errorRate",
            "traces_over_time",
            "tracesOverTime",
            "cost_over_time",
            "costOverTime",
            "tokens_over_time",
            "tokensOverTime",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            TotalTraces,
            TotalSpans,
            TotalLlmCalls,
            TotalInputTokens,
            TotalOutputTokens,
            TotalCostUsd,
            AvgLatencyMs,
            ErrorRate,
            TracesOverTime,
            CostOverTime,
            TokensOverTime,
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
                            "totalTraces" | "total_traces" => Ok(GeneratedField::TotalTraces),
                            "totalSpans" | "total_spans" => Ok(GeneratedField::TotalSpans),
                            "totalLlmCalls" | "total_llm_calls" => Ok(GeneratedField::TotalLlmCalls),
                            "totalInputTokens" | "total_input_tokens" => Ok(GeneratedField::TotalInputTokens),
                            "totalOutputTokens" | "total_output_tokens" => Ok(GeneratedField::TotalOutputTokens),
                            "totalCostUsd" | "total_cost_usd" => Ok(GeneratedField::TotalCostUsd),
                            "avgLatencyMs" | "avg_latency_ms" => Ok(GeneratedField::AvgLatencyMs),
                            "errorRate" | "error_rate" => Ok(GeneratedField::ErrorRate),
                            "tracesOverTime" | "traces_over_time" => Ok(GeneratedField::TracesOverTime),
                            "costOverTime" | "cost_over_time" => Ok(GeneratedField::CostOverTime),
                            "tokensOverTime" | "tokens_over_time" => Ok(GeneratedField::TokensOverTime),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = GetUsageSummaryResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.GetUsageSummaryResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<GetUsageSummaryResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut total_traces__ = None;
                let mut total_spans__ = None;
                let mut total_llm_calls__ = None;
                let mut total_input_tokens__ = None;
                let mut total_output_tokens__ = None;
                let mut total_cost_usd__ = None;
                let mut avg_latency_ms__ = None;
                let mut error_rate__ = None;
                let mut traces_over_time__ = None;
                let mut cost_over_time__ = None;
                let mut tokens_over_time__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::TotalTraces => {
                            if total_traces__.is_some() {
                                return Err(serde::de::Error::duplicate_field("totalTraces"));
                            }
                            total_traces__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::TotalSpans => {
                            if total_spans__.is_some() {
                                return Err(serde::de::Error::duplicate_field("totalSpans"));
                            }
                            total_spans__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::TotalLlmCalls => {
                            if total_llm_calls__.is_some() {
                                return Err(serde::de::Error::duplicate_field("totalLlmCalls"));
                            }
                            total_llm_calls__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::TotalInputTokens => {
                            if total_input_tokens__.is_some() {
                                return Err(serde::de::Error::duplicate_field("totalInputTokens"));
                            }
                            total_input_tokens__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::TotalOutputTokens => {
                            if total_output_tokens__.is_some() {
                                return Err(serde::de::Error::duplicate_field("totalOutputTokens"));
                            }
                            total_output_tokens__ =
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
                        GeneratedField::AvgLatencyMs => {
                            if avg_latency_ms__.is_some() {
                                return Err(serde::de::Error::duplicate_field("avgLatencyMs"));
                            }
                            avg_latency_ms__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::ErrorRate => {
                            if error_rate__.is_some() {
                                return Err(serde::de::Error::duplicate_field("errorRate"));
                            }
                            error_rate__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::TracesOverTime => {
                            if traces_over_time__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tracesOverTime"));
                            }
                            traces_over_time__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CostOverTime => {
                            if cost_over_time__.is_some() {
                                return Err(serde::de::Error::duplicate_field("costOverTime"));
                            }
                            cost_over_time__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TokensOverTime => {
                            if tokens_over_time__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tokensOverTime"));
                            }
                            tokens_over_time__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(GetUsageSummaryResponse {
                    total_traces: total_traces__.unwrap_or_default(),
                    total_spans: total_spans__.unwrap_or_default(),
                    total_llm_calls: total_llm_calls__.unwrap_or_default(),
                    total_input_tokens: total_input_tokens__.unwrap_or_default(),
                    total_output_tokens: total_output_tokens__.unwrap_or_default(),
                    total_cost_usd: total_cost_usd__.unwrap_or_default(),
                    avg_latency_ms: avg_latency_ms__.unwrap_or_default(),
                    error_rate: error_rate__.unwrap_or_default(),
                    traces_over_time: traces_over_time__.unwrap_or_default(),
                    cost_over_time: cost_over_time__.unwrap_or_default(),
                    tokens_over_time: tokens_over_time__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.GetUsageSummaryResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for GetUserRequest {
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
        let mut struct_ser = serializer.serialize_struct("candela.v1.GetUserRequest", len)?;
        if !self.id.is_empty() {
            struct_ser.serialize_field("id", &self.id)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for GetUserRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "id",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Id,
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
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = GetUserRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.GetUserRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<GetUserRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut id__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Id => {
                            if id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("id"));
                            }
                            id__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(GetUserRequest {
                    id: id__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.GetUserRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for GetUserResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.user.is_some() {
            len += 1;
        }
        if self.budget.is_some() {
            len += 1;
        }
        if !self.active_grants.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.GetUserResponse", len)?;
        if let Some(v) = self.user.as_ref() {
            struct_ser.serialize_field("user", v)?;
        }
        if let Some(v) = self.budget.as_ref() {
            struct_ser.serialize_field("budget", v)?;
        }
        if !self.active_grants.is_empty() {
            struct_ser.serialize_field("activeGrants", &self.active_grants)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for GetUserResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "user",
            "budget",
            "active_grants",
            "activeGrants",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            User,
            Budget,
            ActiveGrants,
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
                            "user" => Ok(GeneratedField::User),
                            "budget" => Ok(GeneratedField::Budget),
                            "activeGrants" | "active_grants" => Ok(GeneratedField::ActiveGrants),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = GetUserResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.GetUserResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<GetUserResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut user__ = None;
                let mut budget__ = None;
                let mut active_grants__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::User => {
                            if user__.is_some() {
                                return Err(serde::de::Error::duplicate_field("user"));
                            }
                            user__ = map_.next_value()?;
                        }
                        GeneratedField::Budget => {
                            if budget__.is_some() {
                                return Err(serde::de::Error::duplicate_field("budget"));
                            }
                            budget__ = map_.next_value()?;
                        }
                        GeneratedField::ActiveGrants => {
                            if active_grants__.is_some() {
                                return Err(serde::de::Error::duplicate_field("activeGrants"));
                            }
                            active_grants__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(GetUserResponse {
                    user: user__,
                    budget: budget__,
                    active_grants: active_grants__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.GetUserResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for IngestSpansRequest {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.spans.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.IngestSpansRequest", len)?;
        if !self.spans.is_empty() {
            struct_ser.serialize_field("spans", &self.spans)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for IngestSpansRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "spans",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
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
            type Value = IngestSpansRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.IngestSpansRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<IngestSpansRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut spans__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Spans => {
                            if spans__.is_some() {
                                return Err(serde::de::Error::duplicate_field("spans"));
                            }
                            spans__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(IngestSpansRequest {
                    spans: spans__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.IngestSpansRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for IngestSpansResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.accepted_count != 0 {
            len += 1;
        }
        if self.rejected_count != 0 {
            len += 1;
        }
        if !self.errors.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.IngestSpansResponse", len)?;
        if self.accepted_count != 0 {
            struct_ser.serialize_field("acceptedCount", &self.accepted_count)?;
        }
        if self.rejected_count != 0 {
            struct_ser.serialize_field("rejectedCount", &self.rejected_count)?;
        }
        if !self.errors.is_empty() {
            struct_ser.serialize_field("errors", &self.errors)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for IngestSpansResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "accepted_count",
            "acceptedCount",
            "rejected_count",
            "rejectedCount",
            "errors",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            AcceptedCount,
            RejectedCount,
            Errors,
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
                            "acceptedCount" | "accepted_count" => Ok(GeneratedField::AcceptedCount),
                            "rejectedCount" | "rejected_count" => Ok(GeneratedField::RejectedCount),
                            "errors" => Ok(GeneratedField::Errors),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = IngestSpansResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.IngestSpansResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<IngestSpansResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut accepted_count__ = None;
                let mut rejected_count__ = None;
                let mut errors__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::AcceptedCount => {
                            if accepted_count__.is_some() {
                                return Err(serde::de::Error::duplicate_field("acceptedCount"));
                            }
                            accepted_count__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::RejectedCount => {
                            if rejected_count__.is_some() {
                                return Err(serde::de::Error::duplicate_field("rejectedCount"));
                            }
                            rejected_count__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Errors => {
                            if errors__.is_some() {
                                return Err(serde::de::Error::duplicate_field("errors"));
                            }
                            errors__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(IngestSpansResponse {
                    accepted_count: accepted_count__.unwrap_or_default(),
                    rejected_count: rejected_count__.unwrap_or_default(),
                    errors: errors__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.IngestSpansResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for ListApiKeysRequest {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.project_id.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.ListAPIKeysRequest", len)?;
        if !self.project_id.is_empty() {
            struct_ser.serialize_field("projectId", &self.project_id)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for ListApiKeysRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "project_id",
            "projectId",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            ProjectId,
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
                            "projectId" | "project_id" => Ok(GeneratedField::ProjectId),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = ListApiKeysRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.ListAPIKeysRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<ListApiKeysRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut project_id__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::ProjectId => {
                            if project_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("projectId"));
                            }
                            project_id__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(ListApiKeysRequest {
                    project_id: project_id__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.ListAPIKeysRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for ListApiKeysResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.api_keys.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.ListAPIKeysResponse", len)?;
        if !self.api_keys.is_empty() {
            struct_ser.serialize_field("apiKeys", &self.api_keys)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for ListApiKeysResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "api_keys",
            "apiKeys",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            ApiKeys,
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
                            "apiKeys" | "api_keys" => Ok(GeneratedField::ApiKeys),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = ListApiKeysResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.ListAPIKeysResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<ListApiKeysResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut api_keys__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::ApiKeys => {
                            if api_keys__.is_some() {
                                return Err(serde::de::Error::duplicate_field("apiKeys"));
                            }
                            api_keys__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(ListApiKeysResponse {
                    api_keys: api_keys__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.ListAPIKeysResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for ListAuditLogRequest {
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
        if self.limit != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.ListAuditLogRequest", len)?;
        if !self.user_id.is_empty() {
            struct_ser.serialize_field("userId", &self.user_id)?;
        }
        if self.limit != 0 {
            struct_ser.serialize_field("limit", &self.limit)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for ListAuditLogRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "user_id",
            "userId",
            "limit",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            UserId,
            Limit,
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
                            "limit" => Ok(GeneratedField::Limit),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = ListAuditLogRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.ListAuditLogRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<ListAuditLogRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut user_id__ = None;
                let mut limit__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::UserId => {
                            if user_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("userId"));
                            }
                            user_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Limit => {
                            if limit__.is_some() {
                                return Err(serde::de::Error::duplicate_field("limit"));
                            }
                            limit__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(ListAuditLogRequest {
                    user_id: user_id__.unwrap_or_default(),
                    limit: limit__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.ListAuditLogRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for ListAuditLogResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.entries.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.ListAuditLogResponse", len)?;
        if !self.entries.is_empty() {
            struct_ser.serialize_field("entries", &self.entries)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for ListAuditLogResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "entries",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Entries,
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
                            "entries" => Ok(GeneratedField::Entries),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = ListAuditLogResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.ListAuditLogResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<ListAuditLogResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut entries__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Entries => {
                            if entries__.is_some() {
                                return Err(serde::de::Error::duplicate_field("entries"));
                            }
                            entries__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(ListAuditLogResponse {
                    entries: entries__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.ListAuditLogResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for ListBackendsRequest {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let len = 0;
        let struct_ser = serializer.serialize_struct("candela.v1.ListBackendsRequest", len)?;
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for ListBackendsRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
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
                            Err(serde::de::Error::unknown_field(value, FIELDS))
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = ListBackendsRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.ListBackendsRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<ListBackendsRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                while map_.next_key::<GeneratedField>()?.is_some() {
                    let _ = map_.next_value::<serde::de::IgnoredAny>()?;
                }
                Ok(ListBackendsRequest {
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.ListBackendsRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for ListBackendsResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.backends.is_empty() {
            len += 1;
        }
        if !self.active.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.ListBackendsResponse", len)?;
        if !self.backends.is_empty() {
            struct_ser.serialize_field("backends", &self.backends)?;
        }
        if !self.active.is_empty() {
            struct_ser.serialize_field("active", &self.active)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for ListBackendsResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "backends",
            "active",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Backends,
            Active,
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
                            "backends" => Ok(GeneratedField::Backends),
                            "active" => Ok(GeneratedField::Active),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = ListBackendsResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.ListBackendsResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<ListBackendsResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut backends__ = None;
                let mut active__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Backends => {
                            if backends__.is_some() {
                                return Err(serde::de::Error::duplicate_field("backends"));
                            }
                            backends__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Active => {
                            if active__.is_some() {
                                return Err(serde::de::Error::duplicate_field("active"));
                            }
                            active__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(ListBackendsResponse {
                    backends: backends__.unwrap_or_default(),
                    active: active__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.ListBackendsResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for ListCatalogRequest {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let len = 0;
        let struct_ser = serializer.serialize_struct("candela.v1.ListCatalogRequest", len)?;
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for ListCatalogRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
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
                            Err(serde::de::Error::unknown_field(value, FIELDS))
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = ListCatalogRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.ListCatalogRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<ListCatalogRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                while map_.next_key::<GeneratedField>()?.is_some() {
                    let _ = map_.next_value::<serde::de::IgnoredAny>()?;
                }
                Ok(ListCatalogRequest {
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.ListCatalogRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for ListCatalogResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.models.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.ListCatalogResponse", len)?;
        if !self.models.is_empty() {
            struct_ser.serialize_field("models", &self.models)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for ListCatalogResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "models",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Models,
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
                            "models" => Ok(GeneratedField::Models),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = ListCatalogResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.ListCatalogResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<ListCatalogResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut models__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Models => {
                            if models__.is_some() {
                                return Err(serde::de::Error::duplicate_field("models"));
                            }
                            models__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(ListCatalogResponse {
                    models: models__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.ListCatalogResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for ListGrantsRequest {
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
        if self.active_only {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.ListGrantsRequest", len)?;
        if !self.user_id.is_empty() {
            struct_ser.serialize_field("userId", &self.user_id)?;
        }
        if self.active_only {
            struct_ser.serialize_field("activeOnly", &self.active_only)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for ListGrantsRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "user_id",
            "userId",
            "active_only",
            "activeOnly",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            UserId,
            ActiveOnly,
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
                            "activeOnly" | "active_only" => Ok(GeneratedField::ActiveOnly),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = ListGrantsRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.ListGrantsRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<ListGrantsRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut user_id__ = None;
                let mut active_only__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::UserId => {
                            if user_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("userId"));
                            }
                            user_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ActiveOnly => {
                            if active_only__.is_some() {
                                return Err(serde::de::Error::duplicate_field("activeOnly"));
                            }
                            active_only__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(ListGrantsRequest {
                    user_id: user_id__.unwrap_or_default(),
                    active_only: active_only__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.ListGrantsRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for ListGrantsResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.grants.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.ListGrantsResponse", len)?;
        if !self.grants.is_empty() {
            struct_ser.serialize_field("grants", &self.grants)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for ListGrantsResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "grants",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Grants,
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
                            "grants" => Ok(GeneratedField::Grants),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = ListGrantsResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.ListGrantsResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<ListGrantsResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut grants__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Grants => {
                            if grants__.is_some() {
                                return Err(serde::de::Error::duplicate_field("grants"));
                            }
                            grants__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(ListGrantsResponse {
                    grants: grants__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.ListGrantsResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for ListModelsRequest {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let len = 0;
        let struct_ser = serializer.serialize_struct("candela.v1.ListModelsRequest", len)?;
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for ListModelsRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
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
                            Err(serde::de::Error::unknown_field(value, FIELDS))
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = ListModelsRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.ListModelsRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<ListModelsRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                while map_.next_key::<GeneratedField>()?.is_some() {
                    let _ = map_.next_value::<serde::de::IgnoredAny>()?;
                }
                Ok(ListModelsRequest {
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.ListModelsRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for ListModelsResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.models.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.ListModelsResponse", len)?;
        if !self.models.is_empty() {
            struct_ser.serialize_field("models", &self.models)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for ListModelsResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "models",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Models,
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
                            "models" => Ok(GeneratedField::Models),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = ListModelsResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.ListModelsResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<ListModelsResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut models__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Models => {
                            if models__.is_some() {
                                return Err(serde::de::Error::duplicate_field("models"));
                            }
                            models__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(ListModelsResponse {
                    models: models__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.ListModelsResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for ListProjectsRequest {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.pagination.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.ListProjectsRequest", len)?;
        if let Some(v) = self.pagination.as_ref() {
            struct_ser.serialize_field("pagination", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for ListProjectsRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "pagination",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Pagination,
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
                            "pagination" => Ok(GeneratedField::Pagination),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = ListProjectsRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.ListProjectsRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<ListProjectsRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut pagination__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Pagination => {
                            if pagination__.is_some() {
                                return Err(serde::de::Error::duplicate_field("pagination"));
                            }
                            pagination__ = map_.next_value()?;
                        }
                    }
                }
                Ok(ListProjectsRequest {
                    pagination: pagination__,
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.ListProjectsRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for ListProjectsResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.projects.is_empty() {
            len += 1;
        }
        if self.pagination.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.ListProjectsResponse", len)?;
        if !self.projects.is_empty() {
            struct_ser.serialize_field("projects", &self.projects)?;
        }
        if let Some(v) = self.pagination.as_ref() {
            struct_ser.serialize_field("pagination", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for ListProjectsResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "projects",
            "pagination",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Projects,
            Pagination,
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
                            "projects" => Ok(GeneratedField::Projects),
                            "pagination" => Ok(GeneratedField::Pagination),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = ListProjectsResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.ListProjectsResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<ListProjectsResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut projects__ = None;
                let mut pagination__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Projects => {
                            if projects__.is_some() {
                                return Err(serde::de::Error::duplicate_field("projects"));
                            }
                            projects__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Pagination => {
                            if pagination__.is_some() {
                                return Err(serde::de::Error::duplicate_field("pagination"));
                            }
                            pagination__ = map_.next_value()?;
                        }
                    }
                }
                Ok(ListProjectsResponse {
                    projects: projects__.unwrap_or_default(),
                    pagination: pagination__,
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.ListProjectsResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for ListTracesRequest {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.project_id.is_empty() {
            len += 1;
        }
        if !self.environment.is_empty() {
            len += 1;
        }
        if self.time_range.is_some() {
            len += 1;
        }
        if !self.model.is_empty() {
            len += 1;
        }
        if !self.provider.is_empty() {
            len += 1;
        }
        if self.status != 0 {
            len += 1;
        }
        if !self.search.is_empty() {
            len += 1;
        }
        if self.pagination.is_some() {
            len += 1;
        }
        if !self.order_by.is_empty() {
            len += 1;
        }
        if self.descending {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.ListTracesRequest", len)?;
        if !self.project_id.is_empty() {
            struct_ser.serialize_field("projectId", &self.project_id)?;
        }
        if !self.environment.is_empty() {
            struct_ser.serialize_field("environment", &self.environment)?;
        }
        if let Some(v) = self.time_range.as_ref() {
            struct_ser.serialize_field("timeRange", v)?;
        }
        if !self.model.is_empty() {
            struct_ser.serialize_field("model", &self.model)?;
        }
        if !self.provider.is_empty() {
            struct_ser.serialize_field("provider", &self.provider)?;
        }
        if self.status != 0 {
            let v = super::types::SpanStatus::try_from(self.status)
                .map_err(|_| serde::ser::Error::custom(format!("Invalid variant {}", self.status)))?;
            struct_ser.serialize_field("status", &v)?;
        }
        if !self.search.is_empty() {
            struct_ser.serialize_field("search", &self.search)?;
        }
        if let Some(v) = self.pagination.as_ref() {
            struct_ser.serialize_field("pagination", v)?;
        }
        if !self.order_by.is_empty() {
            struct_ser.serialize_field("orderBy", &self.order_by)?;
        }
        if self.descending {
            struct_ser.serialize_field("descending", &self.descending)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for ListTracesRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "project_id",
            "projectId",
            "environment",
            "time_range",
            "timeRange",
            "model",
            "provider",
            "status",
            "search",
            "pagination",
            "order_by",
            "orderBy",
            "descending",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            ProjectId,
            Environment,
            TimeRange,
            Model,
            Provider,
            Status,
            Search,
            Pagination,
            OrderBy,
            Descending,
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
                            "projectId" | "project_id" => Ok(GeneratedField::ProjectId),
                            "environment" => Ok(GeneratedField::Environment),
                            "timeRange" | "time_range" => Ok(GeneratedField::TimeRange),
                            "model" => Ok(GeneratedField::Model),
                            "provider" => Ok(GeneratedField::Provider),
                            "status" => Ok(GeneratedField::Status),
                            "search" => Ok(GeneratedField::Search),
                            "pagination" => Ok(GeneratedField::Pagination),
                            "orderBy" | "order_by" => Ok(GeneratedField::OrderBy),
                            "descending" => Ok(GeneratedField::Descending),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = ListTracesRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.ListTracesRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<ListTracesRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut project_id__ = None;
                let mut environment__ = None;
                let mut time_range__ = None;
                let mut model__ = None;
                let mut provider__ = None;
                let mut status__ = None;
                let mut search__ = None;
                let mut pagination__ = None;
                let mut order_by__ = None;
                let mut descending__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
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
                        GeneratedField::TimeRange => {
                            if time_range__.is_some() {
                                return Err(serde::de::Error::duplicate_field("timeRange"));
                            }
                            time_range__ = map_.next_value()?;
                        }
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
                        GeneratedField::Status => {
                            if status__.is_some() {
                                return Err(serde::de::Error::duplicate_field("status"));
                            }
                            status__ = Some(map_.next_value::<super::types::SpanStatus>()? as i32);
                        }
                        GeneratedField::Search => {
                            if search__.is_some() {
                                return Err(serde::de::Error::duplicate_field("search"));
                            }
                            search__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Pagination => {
                            if pagination__.is_some() {
                                return Err(serde::de::Error::duplicate_field("pagination"));
                            }
                            pagination__ = map_.next_value()?;
                        }
                        GeneratedField::OrderBy => {
                            if order_by__.is_some() {
                                return Err(serde::de::Error::duplicate_field("orderBy"));
                            }
                            order_by__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Descending => {
                            if descending__.is_some() {
                                return Err(serde::de::Error::duplicate_field("descending"));
                            }
                            descending__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(ListTracesRequest {
                    project_id: project_id__.unwrap_or_default(),
                    environment: environment__.unwrap_or_default(),
                    time_range: time_range__,
                    model: model__.unwrap_or_default(),
                    provider: provider__.unwrap_or_default(),
                    status: status__.unwrap_or_default(),
                    search: search__.unwrap_or_default(),
                    pagination: pagination__,
                    order_by: order_by__.unwrap_or_default(),
                    descending: descending__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.ListTracesRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for ListTracesResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.traces.is_empty() {
            len += 1;
        }
        if self.pagination.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.ListTracesResponse", len)?;
        if !self.traces.is_empty() {
            struct_ser.serialize_field("traces", &self.traces)?;
        }
        if let Some(v) = self.pagination.as_ref() {
            struct_ser.serialize_field("pagination", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for ListTracesResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "traces",
            "pagination",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Traces,
            Pagination,
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
                            "traces" => Ok(GeneratedField::Traces),
                            "pagination" => Ok(GeneratedField::Pagination),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = ListTracesResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.ListTracesResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<ListTracesResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut traces__ = None;
                let mut pagination__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Traces => {
                            if traces__.is_some() {
                                return Err(serde::de::Error::duplicate_field("traces"));
                            }
                            traces__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Pagination => {
                            if pagination__.is_some() {
                                return Err(serde::de::Error::duplicate_field("pagination"));
                            }
                            pagination__ = map_.next_value()?;
                        }
                    }
                }
                Ok(ListTracesResponse {
                    traces: traces__.unwrap_or_default(),
                    pagination: pagination__,
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.ListTracesResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for ListUsersRequest {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.pagination.is_some() {
            len += 1;
        }
        if self.status_filter != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.ListUsersRequest", len)?;
        if let Some(v) = self.pagination.as_ref() {
            struct_ser.serialize_field("pagination", v)?;
        }
        if self.status_filter != 0 {
            let v = super::types::UserStatus::try_from(self.status_filter)
                .map_err(|_| serde::ser::Error::custom(format!("Invalid variant {}", self.status_filter)))?;
            struct_ser.serialize_field("statusFilter", &v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for ListUsersRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "pagination",
            "status_filter",
            "statusFilter",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Pagination,
            StatusFilter,
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
                            "pagination" => Ok(GeneratedField::Pagination),
                            "statusFilter" | "status_filter" => Ok(GeneratedField::StatusFilter),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = ListUsersRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.ListUsersRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<ListUsersRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut pagination__ = None;
                let mut status_filter__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Pagination => {
                            if pagination__.is_some() {
                                return Err(serde::de::Error::duplicate_field("pagination"));
                            }
                            pagination__ = map_.next_value()?;
                        }
                        GeneratedField::StatusFilter => {
                            if status_filter__.is_some() {
                                return Err(serde::de::Error::duplicate_field("statusFilter"));
                            }
                            status_filter__ = Some(map_.next_value::<super::types::UserStatus>()? as i32);
                        }
                    }
                }
                Ok(ListUsersRequest {
                    pagination: pagination__,
                    status_filter: status_filter__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.ListUsersRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for ListUsersResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.users.is_empty() {
            len += 1;
        }
        if self.pagination.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.ListUsersResponse", len)?;
        if !self.users.is_empty() {
            struct_ser.serialize_field("users", &self.users)?;
        }
        if let Some(v) = self.pagination.as_ref() {
            struct_ser.serialize_field("pagination", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for ListUsersResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "users",
            "pagination",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Users,
            Pagination,
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
                            "users" => Ok(GeneratedField::Users),
                            "pagination" => Ok(GeneratedField::Pagination),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = ListUsersResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.ListUsersResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<ListUsersResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut users__ = None;
                let mut pagination__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Users => {
                            if users__.is_some() {
                                return Err(serde::de::Error::duplicate_field("users"));
                            }
                            users__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Pagination => {
                            if pagination__.is_some() {
                                return Err(serde::de::Error::duplicate_field("pagination"));
                            }
                            pagination__ = map_.next_value()?;
                        }
                    }
                }
                Ok(ListUsersResponse {
                    users: users__.unwrap_or_default(),
                    pagination: pagination__,
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.ListUsersResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for LoadModelRequest {
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
        let mut struct_ser = serializer.serialize_struct("candela.v1.LoadModelRequest", len)?;
        if !self.model.is_empty() {
            struct_ser.serialize_field("model", &self.model)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for LoadModelRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "model",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Model,
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
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = LoadModelRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.LoadModelRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<LoadModelRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut model__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Model => {
                            if model__.is_some() {
                                return Err(serde::de::Error::duplicate_field("model"));
                            }
                            model__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(LoadModelRequest {
                    model: model__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.LoadModelRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for LoadModelResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.state != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.LoadModelResponse", len)?;
        if self.state != 0 {
            let v = ModelLoadState::try_from(self.state)
                .map_err(|_| serde::ser::Error::custom(format!("Invalid variant {}", self.state)))?;
            struct_ser.serialize_field("state", &v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for LoadModelResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "state",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            State,
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
                            "state" => Ok(GeneratedField::State),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = LoadModelResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.LoadModelResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<LoadModelResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut state__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::State => {
                            if state__.is_some() {
                                return Err(serde::de::Error::duplicate_field("state"));
                            }
                            state__ = Some(map_.next_value::<ModelLoadState>()? as i32);
                        }
                    }
                }
                Ok(LoadModelResponse {
                    state: state__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.LoadModelResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for ModelLoadState {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        let variant = match self {
            Self::Unspecified => "MODEL_LOAD_STATE_UNSPECIFIED",
            Self::Loaded => "MODEL_LOAD_STATE_LOADED",
            Self::Starting => "MODEL_LOAD_STATE_STARTING",
        };
        serializer.serialize_str(variant)
    }
}
impl<'de> serde::Deserialize<'de> for ModelLoadState {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "MODEL_LOAD_STATE_UNSPECIFIED",
            "MODEL_LOAD_STATE_LOADED",
            "MODEL_LOAD_STATE_STARTING",
        ];

        struct GeneratedVisitor;

        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = ModelLoadState;

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
                    "MODEL_LOAD_STATE_UNSPECIFIED" => Ok(ModelLoadState::Unspecified),
                    "MODEL_LOAD_STATE_LOADED" => Ok(ModelLoadState::Loaded),
                    "MODEL_LOAD_STATE_STARTING" => Ok(ModelLoadState::Starting),
                    _ => Err(serde::de::Error::unknown_variant(value, FIELDS)),
                }
            }
        }
        deserializer.deserialize_any(GeneratedVisitor)
    }
}
impl serde::Serialize for ModelUsage {
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
        if self.call_count != 0 {
            len += 1;
        }
        if self.input_tokens != 0 {
            len += 1;
        }
        if self.output_tokens != 0 {
            len += 1;
        }
        if self.cost_usd != 0. {
            len += 1;
        }
        if self.avg_latency_ms != 0. {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.ModelUsage", len)?;
        if !self.model.is_empty() {
            struct_ser.serialize_field("model", &self.model)?;
        }
        if !self.provider.is_empty() {
            struct_ser.serialize_field("provider", &self.provider)?;
        }
        if self.call_count != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("callCount", ToString::to_string(&self.call_count).as_str())?;
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
        if self.cost_usd != 0. {
            struct_ser.serialize_field("costUsd", &self.cost_usd)?;
        }
        if self.avg_latency_ms != 0. {
            struct_ser.serialize_field("avgLatencyMs", &self.avg_latency_ms)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for ModelUsage {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "model",
            "provider",
            "call_count",
            "callCount",
            "input_tokens",
            "inputTokens",
            "output_tokens",
            "outputTokens",
            "cost_usd",
            "costUsd",
            "avg_latency_ms",
            "avgLatencyMs",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Model,
            Provider,
            CallCount,
            InputTokens,
            OutputTokens,
            CostUsd,
            AvgLatencyMs,
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
                            "callCount" | "call_count" => Ok(GeneratedField::CallCount),
                            "inputTokens" | "input_tokens" => Ok(GeneratedField::InputTokens),
                            "outputTokens" | "output_tokens" => Ok(GeneratedField::OutputTokens),
                            "costUsd" | "cost_usd" => Ok(GeneratedField::CostUsd),
                            "avgLatencyMs" | "avg_latency_ms" => Ok(GeneratedField::AvgLatencyMs),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = ModelUsage;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.ModelUsage")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<ModelUsage, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut model__ = None;
                let mut provider__ = None;
                let mut call_count__ = None;
                let mut input_tokens__ = None;
                let mut output_tokens__ = None;
                let mut cost_usd__ = None;
                let mut avg_latency_ms__ = None;
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
                        GeneratedField::CallCount => {
                            if call_count__.is_some() {
                                return Err(serde::de::Error::duplicate_field("callCount"));
                            }
                            call_count__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
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
                        GeneratedField::CostUsd => {
                            if cost_usd__.is_some() {
                                return Err(serde::de::Error::duplicate_field("costUsd"));
                            }
                            cost_usd__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::AvgLatencyMs => {
                            if avg_latency_ms__.is_some() {
                                return Err(serde::de::Error::duplicate_field("avgLatencyMs"));
                            }
                            avg_latency_ms__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(ModelUsage {
                    model: model__.unwrap_or_default(),
                    provider: provider__.unwrap_or_default(),
                    call_count: call_count__.unwrap_or_default(),
                    input_tokens: input_tokens__.unwrap_or_default(),
                    output_tokens: output_tokens__.unwrap_or_default(),
                    cost_usd: cost_usd__.unwrap_or_default(),
                    avg_latency_ms: avg_latency_ms__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.ModelUsage", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for PullModelRequest {
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
        let mut struct_ser = serializer.serialize_struct("candela.v1.PullModelRequest", len)?;
        if !self.model.is_empty() {
            struct_ser.serialize_field("model", &self.model)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for PullModelRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "model",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Model,
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
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = PullModelRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.PullModelRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<PullModelRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut model__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Model => {
                            if model__.is_some() {
                                return Err(serde::de::Error::duplicate_field("model"));
                            }
                            model__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(PullModelRequest {
                    model: model__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.PullModelRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for PullModelResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let len = 0;
        let struct_ser = serializer.serialize_struct("candela.v1.PullModelResponse", len)?;
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for PullModelResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
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
                            Err(serde::de::Error::unknown_field(value, FIELDS))
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = PullModelResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.PullModelResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<PullModelResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                while map_.next_key::<GeneratedField>()?.is_some() {
                    let _ = map_.next_value::<serde::de::IgnoredAny>()?;
                }
                Ok(PullModelResponse {
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.PullModelResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for ReactivateUserRequest {
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
        let mut struct_ser = serializer.serialize_struct("candela.v1.ReactivateUserRequest", len)?;
        if !self.id.is_empty() {
            struct_ser.serialize_field("id", &self.id)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for ReactivateUserRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "id",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Id,
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
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = ReactivateUserRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.ReactivateUserRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<ReactivateUserRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut id__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Id => {
                            if id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("id"));
                            }
                            id__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(ReactivateUserRequest {
                    id: id__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.ReactivateUserRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for ReactivateUserResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.user.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.ReactivateUserResponse", len)?;
        if let Some(v) = self.user.as_ref() {
            struct_ser.serialize_field("user", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for ReactivateUserResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "user",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            User,
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
                            "user" => Ok(GeneratedField::User),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = ReactivateUserResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.ReactivateUserResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<ReactivateUserResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut user__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::User => {
                            if user__.is_some() {
                                return Err(serde::de::Error::duplicate_field("user"));
                            }
                            user__ = map_.next_value()?;
                        }
                    }
                }
                Ok(ReactivateUserResponse {
                    user: user__,
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.ReactivateUserResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for ResetSpendRequest {
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
        let mut struct_ser = serializer.serialize_struct("candela.v1.ResetSpendRequest", len)?;
        if !self.user_id.is_empty() {
            struct_ser.serialize_field("userId", &self.user_id)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for ResetSpendRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "user_id",
            "userId",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
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
            type Value = ResetSpendRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.ResetSpendRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<ResetSpendRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut user_id__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::UserId => {
                            if user_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("userId"));
                            }
                            user_id__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(ResetSpendRequest {
                    user_id: user_id__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.ResetSpendRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for ResetSpendResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.budget.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.ResetSpendResponse", len)?;
        if let Some(v) = self.budget.as_ref() {
            struct_ser.serialize_field("budget", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for ResetSpendResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "budget",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Budget,
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
                            "budget" => Ok(GeneratedField::Budget),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = ResetSpendResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.ResetSpendResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<ResetSpendResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut budget__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Budget => {
                            if budget__.is_some() {
                                return Err(serde::de::Error::duplicate_field("budget"));
                            }
                            budget__ = map_.next_value()?;
                        }
                    }
                }
                Ok(ResetSpendResponse {
                    budget: budget__,
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.ResetSpendResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for ResetStateRequest {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let len = 0;
        let struct_ser = serializer.serialize_struct("candela.v1.ResetStateRequest", len)?;
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for ResetStateRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
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
                            Err(serde::de::Error::unknown_field(value, FIELDS))
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = ResetStateRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.ResetStateRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<ResetStateRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                while map_.next_key::<GeneratedField>()?.is_some() {
                    let _ = map_.next_value::<serde::de::IgnoredAny>()?;
                }
                Ok(ResetStateRequest {
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.ResetStateRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for ResetStateResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let len = 0;
        let struct_ser = serializer.serialize_struct("candela.v1.ResetStateResponse", len)?;
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for ResetStateResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
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
                            Err(serde::de::Error::unknown_field(value, FIELDS))
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = ResetStateResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.ResetStateResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<ResetStateResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                while map_.next_key::<GeneratedField>()?.is_some() {
                    let _ = map_.next_value::<serde::de::IgnoredAny>()?;
                }
                Ok(ResetStateResponse {
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.ResetStateResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for RevokeApiKeyRequest {
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
        let mut struct_ser = serializer.serialize_struct("candela.v1.RevokeAPIKeyRequest", len)?;
        if !self.id.is_empty() {
            struct_ser.serialize_field("id", &self.id)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for RevokeApiKeyRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "id",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Id,
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
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = RevokeApiKeyRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.RevokeAPIKeyRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<RevokeApiKeyRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut id__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Id => {
                            if id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("id"));
                            }
                            id__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(RevokeApiKeyRequest {
                    id: id__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.RevokeAPIKeyRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for RevokeApiKeyResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let len = 0;
        let struct_ser = serializer.serialize_struct("candela.v1.RevokeAPIKeyResponse", len)?;
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for RevokeApiKeyResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
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
                            Err(serde::de::Error::unknown_field(value, FIELDS))
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = RevokeApiKeyResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.RevokeAPIKeyResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<RevokeApiKeyResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                while map_.next_key::<GeneratedField>()?.is_some() {
                    let _ = map_.next_value::<serde::de::IgnoredAny>()?;
                }
                Ok(RevokeApiKeyResponse {
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.RevokeAPIKeyResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for RevokeGrantRequest {
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
        if !self.grant_id.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.RevokeGrantRequest", len)?;
        if !self.user_id.is_empty() {
            struct_ser.serialize_field("userId", &self.user_id)?;
        }
        if !self.grant_id.is_empty() {
            struct_ser.serialize_field("grantId", &self.grant_id)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for RevokeGrantRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "user_id",
            "userId",
            "grant_id",
            "grantId",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            UserId,
            GrantId,
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
                            "grantId" | "grant_id" => Ok(GeneratedField::GrantId),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = RevokeGrantRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.RevokeGrantRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<RevokeGrantRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut user_id__ = None;
                let mut grant_id__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::UserId => {
                            if user_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("userId"));
                            }
                            user_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::GrantId => {
                            if grant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("grantId"));
                            }
                            grant_id__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(RevokeGrantRequest {
                    user_id: user_id__.unwrap_or_default(),
                    grant_id: grant_id__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.RevokeGrantRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for RevokeGrantResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let len = 0;
        let struct_ser = serializer.serialize_struct("candela.v1.RevokeGrantResponse", len)?;
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for RevokeGrantResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
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
                            Err(serde::de::Error::unknown_field(value, FIELDS))
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = RevokeGrantResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.RevokeGrantResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<RevokeGrantResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                while map_.next_key::<GeneratedField>()?.is_some() {
                    let _ = map_.next_value::<serde::de::IgnoredAny>()?;
                }
                Ok(RevokeGrantResponse {
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.RevokeGrantResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for RuntimeModel {
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
        if self.size_bytes != 0 {
            len += 1;
        }
        if !self.family.is_empty() {
            len += 1;
        }
        if !self.parameters.is_empty() {
            len += 1;
        }
        if !self.quantization.is_empty() {
            len += 1;
        }
        if self.loaded {
            len += 1;
        }
        if self.available {
            len += 1;
        }
        if self.last_used.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.RuntimeModel", len)?;
        if !self.id.is_empty() {
            struct_ser.serialize_field("id", &self.id)?;
        }
        if self.size_bytes != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("sizeBytes", ToString::to_string(&self.size_bytes).as_str())?;
        }
        if !self.family.is_empty() {
            struct_ser.serialize_field("family", &self.family)?;
        }
        if !self.parameters.is_empty() {
            struct_ser.serialize_field("parameters", &self.parameters)?;
        }
        if !self.quantization.is_empty() {
            struct_ser.serialize_field("quantization", &self.quantization)?;
        }
        if self.loaded {
            struct_ser.serialize_field("loaded", &self.loaded)?;
        }
        if self.available {
            struct_ser.serialize_field("available", &self.available)?;
        }
        if let Some(v) = self.last_used.as_ref() {
            struct_ser.serialize_field("lastUsed", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for RuntimeModel {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "id",
            "size_bytes",
            "sizeBytes",
            "family",
            "parameters",
            "quantization",
            "loaded",
            "available",
            "last_used",
            "lastUsed",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Id,
            SizeBytes,
            Family,
            Parameters,
            Quantization,
            Loaded,
            Available,
            LastUsed,
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
                            "sizeBytes" | "size_bytes" => Ok(GeneratedField::SizeBytes),
                            "family" => Ok(GeneratedField::Family),
                            "parameters" => Ok(GeneratedField::Parameters),
                            "quantization" => Ok(GeneratedField::Quantization),
                            "loaded" => Ok(GeneratedField::Loaded),
                            "available" => Ok(GeneratedField::Available),
                            "lastUsed" | "last_used" => Ok(GeneratedField::LastUsed),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = RuntimeModel;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.RuntimeModel")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<RuntimeModel, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut id__ = None;
                let mut size_bytes__ = None;
                let mut family__ = None;
                let mut parameters__ = None;
                let mut quantization__ = None;
                let mut loaded__ = None;
                let mut available__ = None;
                let mut last_used__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Id => {
                            if id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("id"));
                            }
                            id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::SizeBytes => {
                            if size_bytes__.is_some() {
                                return Err(serde::de::Error::duplicate_field("sizeBytes"));
                            }
                            size_bytes__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Family => {
                            if family__.is_some() {
                                return Err(serde::de::Error::duplicate_field("family"));
                            }
                            family__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Parameters => {
                            if parameters__.is_some() {
                                return Err(serde::de::Error::duplicate_field("parameters"));
                            }
                            parameters__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Quantization => {
                            if quantization__.is_some() {
                                return Err(serde::de::Error::duplicate_field("quantization"));
                            }
                            quantization__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Loaded => {
                            if loaded__.is_some() {
                                return Err(serde::de::Error::duplicate_field("loaded"));
                            }
                            loaded__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Available => {
                            if available__.is_some() {
                                return Err(serde::de::Error::duplicate_field("available"));
                            }
                            available__ = Some(map_.next_value()?);
                        }
                        GeneratedField::LastUsed => {
                            if last_used__.is_some() {
                                return Err(serde::de::Error::duplicate_field("lastUsed"));
                            }
                            last_used__ = map_.next_value()?;
                        }
                    }
                }
                Ok(RuntimeModel {
                    id: id__.unwrap_or_default(),
                    size_bytes: size_bytes__.unwrap_or_default(),
                    family: family__.unwrap_or_default(),
                    parameters: parameters__.unwrap_or_default(),
                    quantization: quantization__.unwrap_or_default(),
                    loaded: loaded__.unwrap_or_default(),
                    available: available__.unwrap_or_default(),
                    last_used: last_used__,
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.RuntimeModel", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for RuntimeState {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        let variant = match self {
            Self::Unspecified => "RUNTIME_STATE_UNSPECIFIED",
            Self::Stopped => "RUNTIME_STATE_STOPPED",
            Self::Starting => "RUNTIME_STATE_STARTING",
            Self::Running => "RUNTIME_STATE_RUNNING",
            Self::Error => "RUNTIME_STATE_ERROR",
        };
        serializer.serialize_str(variant)
    }
}
impl<'de> serde::Deserialize<'de> for RuntimeState {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "RUNTIME_STATE_UNSPECIFIED",
            "RUNTIME_STATE_STOPPED",
            "RUNTIME_STATE_STARTING",
            "RUNTIME_STATE_RUNNING",
            "RUNTIME_STATE_ERROR",
        ];

        struct GeneratedVisitor;

        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = RuntimeState;

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
                    "RUNTIME_STATE_UNSPECIFIED" => Ok(RuntimeState::Unspecified),
                    "RUNTIME_STATE_STOPPED" => Ok(RuntimeState::Stopped),
                    "RUNTIME_STATE_STARTING" => Ok(RuntimeState::Starting),
                    "RUNTIME_STATE_RUNNING" => Ok(RuntimeState::Running),
                    "RUNTIME_STATE_ERROR" => Ok(RuntimeState::Error),
                    _ => Err(serde::de::Error::unknown_variant(value, FIELDS)),
                }
            }
        }
        deserializer.deserialize_any(GeneratedVisitor)
    }
}
impl serde::Serialize for RuntimeStatus {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.state != 0 {
            len += 1;
        }
        if !self.backend.is_empty() {
            len += 1;
        }
        if !self.endpoint.is_empty() {
            len += 1;
        }
        if self.uptime_seconds != 0. {
            len += 1;
        }
        if !self.error.is_empty() {
            len += 1;
        }
        if self.checked_at.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.RuntimeStatus", len)?;
        if self.state != 0 {
            let v = RuntimeState::try_from(self.state)
                .map_err(|_| serde::ser::Error::custom(format!("Invalid variant {}", self.state)))?;
            struct_ser.serialize_field("state", &v)?;
        }
        if !self.backend.is_empty() {
            struct_ser.serialize_field("backend", &self.backend)?;
        }
        if !self.endpoint.is_empty() {
            struct_ser.serialize_field("endpoint", &self.endpoint)?;
        }
        if self.uptime_seconds != 0. {
            struct_ser.serialize_field("uptimeSeconds", &self.uptime_seconds)?;
        }
        if !self.error.is_empty() {
            struct_ser.serialize_field("error", &self.error)?;
        }
        if let Some(v) = self.checked_at.as_ref() {
            struct_ser.serialize_field("checkedAt", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for RuntimeStatus {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "state",
            "backend",
            "endpoint",
            "uptime_seconds",
            "uptimeSeconds",
            "error",
            "checked_at",
            "checkedAt",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            State,
            Backend,
            Endpoint,
            UptimeSeconds,
            Error,
            CheckedAt,
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
                            "state" => Ok(GeneratedField::State),
                            "backend" => Ok(GeneratedField::Backend),
                            "endpoint" => Ok(GeneratedField::Endpoint),
                            "uptimeSeconds" | "uptime_seconds" => Ok(GeneratedField::UptimeSeconds),
                            "error" => Ok(GeneratedField::Error),
                            "checkedAt" | "checked_at" => Ok(GeneratedField::CheckedAt),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = RuntimeStatus;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.RuntimeStatus")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<RuntimeStatus, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut state__ = None;
                let mut backend__ = None;
                let mut endpoint__ = None;
                let mut uptime_seconds__ = None;
                let mut error__ = None;
                let mut checked_at__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::State => {
                            if state__.is_some() {
                                return Err(serde::de::Error::duplicate_field("state"));
                            }
                            state__ = Some(map_.next_value::<RuntimeState>()? as i32);
                        }
                        GeneratedField::Backend => {
                            if backend__.is_some() {
                                return Err(serde::de::Error::duplicate_field("backend"));
                            }
                            backend__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Endpoint => {
                            if endpoint__.is_some() {
                                return Err(serde::de::Error::duplicate_field("endpoint"));
                            }
                            endpoint__ = Some(map_.next_value()?);
                        }
                        GeneratedField::UptimeSeconds => {
                            if uptime_seconds__.is_some() {
                                return Err(serde::de::Error::duplicate_field("uptimeSeconds"));
                            }
                            uptime_seconds__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Error => {
                            if error__.is_some() {
                                return Err(serde::de::Error::duplicate_field("error"));
                            }
                            error__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CheckedAt => {
                            if checked_at__.is_some() {
                                return Err(serde::de::Error::duplicate_field("checkedAt"));
                            }
                            checked_at__ = map_.next_value()?;
                        }
                    }
                }
                Ok(RuntimeStatus {
                    state: state__.unwrap_or_default(),
                    backend: backend__.unwrap_or_default(),
                    endpoint: endpoint__.unwrap_or_default(),
                    uptime_seconds: uptime_seconds__.unwrap_or_default(),
                    error: error__.unwrap_or_default(),
                    checked_at: checked_at__,
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.RuntimeStatus", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for SearchSpansRequest {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.project_id.is_empty() {
            len += 1;
        }
        if self.time_range.is_some() {
            len += 1;
        }
        if self.kind != 0 {
            len += 1;
        }
        if !self.model.is_empty() {
            len += 1;
        }
        if !self.name_contains.is_empty() {
            len += 1;
        }
        if self.pagination.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.SearchSpansRequest", len)?;
        if !self.project_id.is_empty() {
            struct_ser.serialize_field("projectId", &self.project_id)?;
        }
        if let Some(v) = self.time_range.as_ref() {
            struct_ser.serialize_field("timeRange", v)?;
        }
        if self.kind != 0 {
            let v = super::types::SpanKind::try_from(self.kind)
                .map_err(|_| serde::ser::Error::custom(format!("Invalid variant {}", self.kind)))?;
            struct_ser.serialize_field("kind", &v)?;
        }
        if !self.model.is_empty() {
            struct_ser.serialize_field("model", &self.model)?;
        }
        if !self.name_contains.is_empty() {
            struct_ser.serialize_field("nameContains", &self.name_contains)?;
        }
        if let Some(v) = self.pagination.as_ref() {
            struct_ser.serialize_field("pagination", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for SearchSpansRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "project_id",
            "projectId",
            "time_range",
            "timeRange",
            "kind",
            "model",
            "name_contains",
            "nameContains",
            "pagination",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            ProjectId,
            TimeRange,
            Kind,
            Model,
            NameContains,
            Pagination,
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
                            "projectId" | "project_id" => Ok(GeneratedField::ProjectId),
                            "timeRange" | "time_range" => Ok(GeneratedField::TimeRange),
                            "kind" => Ok(GeneratedField::Kind),
                            "model" => Ok(GeneratedField::Model),
                            "nameContains" | "name_contains" => Ok(GeneratedField::NameContains),
                            "pagination" => Ok(GeneratedField::Pagination),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = SearchSpansRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.SearchSpansRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<SearchSpansRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut project_id__ = None;
                let mut time_range__ = None;
                let mut kind__ = None;
                let mut model__ = None;
                let mut name_contains__ = None;
                let mut pagination__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::ProjectId => {
                            if project_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("projectId"));
                            }
                            project_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TimeRange => {
                            if time_range__.is_some() {
                                return Err(serde::de::Error::duplicate_field("timeRange"));
                            }
                            time_range__ = map_.next_value()?;
                        }
                        GeneratedField::Kind => {
                            if kind__.is_some() {
                                return Err(serde::de::Error::duplicate_field("kind"));
                            }
                            kind__ = Some(map_.next_value::<super::types::SpanKind>()? as i32);
                        }
                        GeneratedField::Model => {
                            if model__.is_some() {
                                return Err(serde::de::Error::duplicate_field("model"));
                            }
                            model__ = Some(map_.next_value()?);
                        }
                        GeneratedField::NameContains => {
                            if name_contains__.is_some() {
                                return Err(serde::de::Error::duplicate_field("nameContains"));
                            }
                            name_contains__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Pagination => {
                            if pagination__.is_some() {
                                return Err(serde::de::Error::duplicate_field("pagination"));
                            }
                            pagination__ = map_.next_value()?;
                        }
                    }
                }
                Ok(SearchSpansRequest {
                    project_id: project_id__.unwrap_or_default(),
                    time_range: time_range__,
                    kind: kind__.unwrap_or_default(),
                    model: model__.unwrap_or_default(),
                    name_contains: name_contains__.unwrap_or_default(),
                    pagination: pagination__,
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.SearchSpansRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for SearchSpansResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.spans.is_empty() {
            len += 1;
        }
        if self.pagination.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.SearchSpansResponse", len)?;
        if !self.spans.is_empty() {
            struct_ser.serialize_field("spans", &self.spans)?;
        }
        if let Some(v) = self.pagination.as_ref() {
            struct_ser.serialize_field("pagination", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for SearchSpansResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "spans",
            "pagination",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Spans,
            Pagination,
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
                            "spans" => Ok(GeneratedField::Spans),
                            "pagination" => Ok(GeneratedField::Pagination),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = SearchSpansResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.SearchSpansResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<SearchSpansResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut spans__ = None;
                let mut pagination__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Spans => {
                            if spans__.is_some() {
                                return Err(serde::de::Error::duplicate_field("spans"));
                            }
                            spans__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Pagination => {
                            if pagination__.is_some() {
                                return Err(serde::de::Error::duplicate_field("pagination"));
                            }
                            pagination__ = map_.next_value()?;
                        }
                    }
                }
                Ok(SearchSpansResponse {
                    spans: spans__.unwrap_or_default(),
                    pagination: pagination__,
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.SearchSpansResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for SetBudgetRequest {
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
        if self.period_type != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.SetBudgetRequest", len)?;
        if !self.user_id.is_empty() {
            struct_ser.serialize_field("userId", &self.user_id)?;
        }
        if self.limit_usd != 0. {
            struct_ser.serialize_field("limitUsd", &self.limit_usd)?;
        }
        if self.period_type != 0 {
            let v = super::types::BudgetPeriod::try_from(self.period_type)
                .map_err(|_| serde::ser::Error::custom(format!("Invalid variant {}", self.period_type)))?;
            struct_ser.serialize_field("periodType", &v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for SetBudgetRequest {
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
            "period_type",
            "periodType",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            UserId,
            LimitUsd,
            PeriodType,
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
                            "periodType" | "period_type" => Ok(GeneratedField::PeriodType),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = SetBudgetRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.SetBudgetRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<SetBudgetRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut user_id__ = None;
                let mut limit_usd__ = None;
                let mut period_type__ = None;
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
                        GeneratedField::PeriodType => {
                            if period_type__.is_some() {
                                return Err(serde::de::Error::duplicate_field("periodType"));
                            }
                            period_type__ = Some(map_.next_value::<super::types::BudgetPeriod>()? as i32);
                        }
                    }
                }
                Ok(SetBudgetRequest {
                    user_id: user_id__.unwrap_or_default(),
                    limit_usd: limit_usd__.unwrap_or_default(),
                    period_type: period_type__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.SetBudgetRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for SetBudgetResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.budget.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.SetBudgetResponse", len)?;
        if let Some(v) = self.budget.as_ref() {
            struct_ser.serialize_field("budget", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for SetBudgetResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "budget",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Budget,
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
                            "budget" => Ok(GeneratedField::Budget),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = SetBudgetResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.SetBudgetResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<SetBudgetResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut budget__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Budget => {
                            if budget__.is_some() {
                                return Err(serde::de::Error::duplicate_field("budget"));
                            }
                            budget__ = map_.next_value()?;
                        }
                    }
                }
                Ok(SetBudgetResponse {
                    budget: budget__,
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.SetBudgetResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for StartRuntimeRequest {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.backend.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.StartRuntimeRequest", len)?;
        if !self.backend.is_empty() {
            struct_ser.serialize_field("backend", &self.backend)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for StartRuntimeRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "backend",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Backend,
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
                            "backend" => Ok(GeneratedField::Backend),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = StartRuntimeRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.StartRuntimeRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<StartRuntimeRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut backend__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Backend => {
                            if backend__.is_some() {
                                return Err(serde::de::Error::duplicate_field("backend"));
                            }
                            backend__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(StartRuntimeRequest {
                    backend: backend__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.StartRuntimeRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for StartRuntimeResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.status.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.StartRuntimeResponse", len)?;
        if let Some(v) = self.status.as_ref() {
            struct_ser.serialize_field("status", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for StartRuntimeResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "status",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Status,
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
                            "status" => Ok(GeneratedField::Status),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = StartRuntimeResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.StartRuntimeResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<StartRuntimeResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut status__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Status => {
                            if status__.is_some() {
                                return Err(serde::de::Error::duplicate_field("status"));
                            }
                            status__ = map_.next_value()?;
                        }
                    }
                }
                Ok(StartRuntimeResponse {
                    status: status__,
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.StartRuntimeResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for StopRuntimeRequest {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let len = 0;
        let struct_ser = serializer.serialize_struct("candela.v1.StopRuntimeRequest", len)?;
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for StopRuntimeRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
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
                            Err(serde::de::Error::unknown_field(value, FIELDS))
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = StopRuntimeRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.StopRuntimeRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<StopRuntimeRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                while map_.next_key::<GeneratedField>()?.is_some() {
                    let _ = map_.next_value::<serde::de::IgnoredAny>()?;
                }
                Ok(StopRuntimeRequest {
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.StopRuntimeRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for StopRuntimeResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.status.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.StopRuntimeResponse", len)?;
        if let Some(v) = self.status.as_ref() {
            struct_ser.serialize_field("status", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for StopRuntimeResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "status",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Status,
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
                            "status" => Ok(GeneratedField::Status),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = StopRuntimeResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.StopRuntimeResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<StopRuntimeResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut status__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Status => {
                            if status__.is_some() {
                                return Err(serde::de::Error::duplicate_field("status"));
                            }
                            status__ = map_.next_value()?;
                        }
                    }
                }
                Ok(StopRuntimeResponse {
                    status: status__,
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.StopRuntimeResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for TimeSeriesPoint {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.timestamp.is_empty() {
            len += 1;
        }
        if self.value != 0. {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.TimeSeriesPoint", len)?;
        if !self.timestamp.is_empty() {
            struct_ser.serialize_field("timestamp", &self.timestamp)?;
        }
        if self.value != 0. {
            struct_ser.serialize_field("value", &self.value)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for TimeSeriesPoint {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "timestamp",
            "value",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Timestamp,
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
                            "timestamp" => Ok(GeneratedField::Timestamp),
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
            type Value = TimeSeriesPoint;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.TimeSeriesPoint")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<TimeSeriesPoint, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut timestamp__ = None;
                let mut value__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Timestamp => {
                            if timestamp__.is_some() {
                                return Err(serde::de::Error::duplicate_field("timestamp"));
                            }
                            timestamp__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Value => {
                            if value__.is_some() {
                                return Err(serde::de::Error::duplicate_field("value"));
                            }
                            value__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(TimeSeriesPoint {
                    timestamp: timestamp__.unwrap_or_default(),
                    value: value__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.TimeSeriesPoint", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for UnloadModelRequest {
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
        let mut struct_ser = serializer.serialize_struct("candela.v1.UnloadModelRequest", len)?;
        if !self.model.is_empty() {
            struct_ser.serialize_field("model", &self.model)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for UnloadModelRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "model",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Model,
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
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = UnloadModelRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.UnloadModelRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<UnloadModelRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut model__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Model => {
                            if model__.is_some() {
                                return Err(serde::de::Error::duplicate_field("model"));
                            }
                            model__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(UnloadModelRequest {
                    model: model__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.UnloadModelRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for UnloadModelResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let len = 0;
        let struct_ser = serializer.serialize_struct("candela.v1.UnloadModelResponse", len)?;
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for UnloadModelResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
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
                            Err(serde::de::Error::unknown_field(value, FIELDS))
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = UnloadModelResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.UnloadModelResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<UnloadModelResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                while map_.next_key::<GeneratedField>()?.is_some() {
                    let _ = map_.next_value::<serde::de::IgnoredAny>()?;
                }
                Ok(UnloadModelResponse {
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.UnloadModelResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for UpdateUserRequest {
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
        if !self.display_name.is_empty() {
            len += 1;
        }
        if self.role != 0 {
            len += 1;
        }
        if self.update_mask.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.UpdateUserRequest", len)?;
        if !self.id.is_empty() {
            struct_ser.serialize_field("id", &self.id)?;
        }
        if !self.display_name.is_empty() {
            struct_ser.serialize_field("displayName", &self.display_name)?;
        }
        if self.role != 0 {
            let v = super::types::UserRole::try_from(self.role)
                .map_err(|_| serde::ser::Error::custom(format!("Invalid variant {}", self.role)))?;
            struct_ser.serialize_field("role", &v)?;
        }
        if let Some(v) = self.update_mask.as_ref() {
            struct_ser.serialize_field("updateMask", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for UpdateUserRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "id",
            "display_name",
            "displayName",
            "role",
            "update_mask",
            "updateMask",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Id,
            DisplayName,
            Role,
            UpdateMask,
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
                            "displayName" | "display_name" => Ok(GeneratedField::DisplayName),
                            "role" => Ok(GeneratedField::Role),
                            "updateMask" | "update_mask" => Ok(GeneratedField::UpdateMask),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = UpdateUserRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.UpdateUserRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<UpdateUserRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut id__ = None;
                let mut display_name__ = None;
                let mut role__ = None;
                let mut update_mask__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Id => {
                            if id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("id"));
                            }
                            id__ = Some(map_.next_value()?);
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
                            role__ = Some(map_.next_value::<super::types::UserRole>()? as i32);
                        }
                        GeneratedField::UpdateMask => {
                            if update_mask__.is_some() {
                                return Err(serde::de::Error::duplicate_field("updateMask"));
                            }
                            update_mask__ = map_.next_value()?;
                        }
                    }
                }
                Ok(UpdateUserRequest {
                    id: id__.unwrap_or_default(),
                    display_name: display_name__.unwrap_or_default(),
                    role: role__.unwrap_or_default(),
                    update_mask: update_mask__,
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.UpdateUserRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for UpdateUserResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.user.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.UpdateUserResponse", len)?;
        if let Some(v) = self.user.as_ref() {
            struct_ser.serialize_field("user", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for UpdateUserResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "user",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            User,
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
                            "user" => Ok(GeneratedField::User),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = UpdateUserResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.UpdateUserResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<UpdateUserResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut user__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::User => {
                            if user__.is_some() {
                                return Err(serde::de::Error::duplicate_field("user"));
                            }
                            user__ = map_.next_value()?;
                        }
                    }
                }
                Ok(UpdateUserResponse {
                    user: user__,
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.UpdateUserResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for UserUsage {
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
        if !self.email.is_empty() {
            len += 1;
        }
        if !self.display_name.is_empty() {
            len += 1;
        }
        if self.call_count != 0 {
            len += 1;
        }
        if self.total_tokens != 0 {
            len += 1;
        }
        if self.cost_usd != 0. {
            len += 1;
        }
        if self.avg_latency_ms != 0. {
            len += 1;
        }
        if !self.top_model.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("candela.v1.UserUsage", len)?;
        if !self.user_id.is_empty() {
            struct_ser.serialize_field("userId", &self.user_id)?;
        }
        if !self.email.is_empty() {
            struct_ser.serialize_field("email", &self.email)?;
        }
        if !self.display_name.is_empty() {
            struct_ser.serialize_field("displayName", &self.display_name)?;
        }
        if self.call_count != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("callCount", ToString::to_string(&self.call_count).as_str())?;
        }
        if self.total_tokens != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("totalTokens", ToString::to_string(&self.total_tokens).as_str())?;
        }
        if self.cost_usd != 0. {
            struct_ser.serialize_field("costUsd", &self.cost_usd)?;
        }
        if self.avg_latency_ms != 0. {
            struct_ser.serialize_field("avgLatencyMs", &self.avg_latency_ms)?;
        }
        if !self.top_model.is_empty() {
            struct_ser.serialize_field("topModel", &self.top_model)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for UserUsage {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "user_id",
            "userId",
            "email",
            "display_name",
            "displayName",
            "call_count",
            "callCount",
            "total_tokens",
            "totalTokens",
            "cost_usd",
            "costUsd",
            "avg_latency_ms",
            "avgLatencyMs",
            "top_model",
            "topModel",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            UserId,
            Email,
            DisplayName,
            CallCount,
            TotalTokens,
            CostUsd,
            AvgLatencyMs,
            TopModel,
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
                            "email" => Ok(GeneratedField::Email),
                            "displayName" | "display_name" => Ok(GeneratedField::DisplayName),
                            "callCount" | "call_count" => Ok(GeneratedField::CallCount),
                            "totalTokens" | "total_tokens" => Ok(GeneratedField::TotalTokens),
                            "costUsd" | "cost_usd" => Ok(GeneratedField::CostUsd),
                            "avgLatencyMs" | "avg_latency_ms" => Ok(GeneratedField::AvgLatencyMs),
                            "topModel" | "top_model" => Ok(GeneratedField::TopModel),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = UserUsage;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct candela.v1.UserUsage")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<UserUsage, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut user_id__ = None;
                let mut email__ = None;
                let mut display_name__ = None;
                let mut call_count__ = None;
                let mut total_tokens__ = None;
                let mut cost_usd__ = None;
                let mut avg_latency_ms__ = None;
                let mut top_model__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::UserId => {
                            if user_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("userId"));
                            }
                            user_id__ = Some(map_.next_value()?);
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
                        GeneratedField::CallCount => {
                            if call_count__.is_some() {
                                return Err(serde::de::Error::duplicate_field("callCount"));
                            }
                            call_count__ =
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
                        GeneratedField::AvgLatencyMs => {
                            if avg_latency_ms__.is_some() {
                                return Err(serde::de::Error::duplicate_field("avgLatencyMs"));
                            }
                            avg_latency_ms__ =
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::TopModel => {
                            if top_model__.is_some() {
                                return Err(serde::de::Error::duplicate_field("topModel"));
                            }
                            top_model__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(UserUsage {
                    user_id: user_id__.unwrap_or_default(),
                    email: email__.unwrap_or_default(),
                    display_name: display_name__.unwrap_or_default(),
                    call_count: call_count__.unwrap_or_default(),
                    total_tokens: total_tokens__.unwrap_or_default(),
                    cost_usd: cost_usd__.unwrap_or_default(),
                    avg_latency_ms: avg_latency_ms__.unwrap_or_default(),
                    top_model: top_model__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("candela.v1.UserUsage", FIELDS, GeneratedVisitor)
    }
}
