use std::fmt;
use std::path::{Path, PathBuf};

use base64::Engine as _;
use buffa::Message as _;
use connectrpc::client::{ClientConfig, Http2Connection, SharedHttp2Connection};
use connectrpc::ErrorCode;
use flavor_proto::flavor::v1::{
    ConflictServiceClient, DaemonServiceClient, DeviceServiceClient, DiagnosticsServiceClient, EventServiceClient, InspectorServiceClient, PreferenceServiceClient, WorkspaceServiceClient,
    FlavorErrorCode, FlavorErrorDetail, NetworkServiceClient,
};

pub use connectrpc::ConnectError;
pub use flavor_proto::flavor::v1 as proto;

pub const PROTOCOL_MAJOR: u32 = 1;

type Transport = SharedHttp2Connection;

#[derive(Clone)]
pub struct FlavorIpcClient {
    pub daemon: DaemonServiceClient<Transport>,
    pub networks: NetworkServiceClient<Transport>,
    pub devices: DeviceServiceClient<Transport>,
    pub diagnostics: DiagnosticsServiceClient<Transport>,
    pub events: EventServiceClient<Transport>,
    pub inspector: InspectorServiceClient<Transport>,
    pub conflicts: ConflictServiceClient<Transport>,
    pub workspaces: WorkspaceServiceClient<Transport>,
    pub preferences: PreferenceServiceClient<Transport>,
}

impl FlavorIpcClient {
    pub fn new(socket: impl Into<PathBuf>) -> Self {
        let authority: http::Uri = "http://flavord".parse().expect("static uri");
        let transport = Http2Connection::lazy_unix(socket, authority.clone()).shared(256);
        let config = || ClientConfig::new(authority.clone());
        Self {
            daemon: DaemonServiceClient::new(transport.clone(), config()),
            networks: NetworkServiceClient::new(transport.clone(), config()),
            devices: DeviceServiceClient::new(transport.clone(), config()),
            diagnostics: DiagnosticsServiceClient::new(transport.clone(), config()),
            events: EventServiceClient::new(transport.clone(), config()),
            inspector: InspectorServiceClient::new(transport.clone(), config()),
            conflicts: ConflictServiceClient::new(transport.clone(), config()),
            workspaces: WorkspaceServiceClient::new(transport.clone(), config()),
            preferences: PreferenceServiceClient::new(transport, config()),
        }
    }
}

pub fn default_socket_path() -> Option<PathBuf> {
    let runtime = std::env::var_os("XDG_RUNTIME_DIR")
        .filter(|p| !p.is_empty())
        .map(PathBuf::from)
        .or_else(macos_support_dir)?;
    runtime
        .is_absolute()
        .then(|| runtime.join("flavor").join("flavord.sock"))
}

pub fn macos_support_dir() -> Option<PathBuf> {
    if !cfg!(target_os = "macos") {
        return None;
    }
    std::env::var_os("HOME").map(|h| Path::new(&h).join("Library").join("Application Support"))
}

#[derive(Debug, Clone)]
pub struct IpcError {
    pub code: ErrorCode,
    pub flavor: Option<FlavorErrorDetail>,
    pub message: String,
}

impl IpcError {
    pub fn flavor_code(&self) -> Option<FlavorErrorCode> {
        self.flavor.as_ref().and_then(|d| d.code.as_known())
    }

    pub fn is_resync_required(&self) -> bool {
        self.flavor_code() == Some(FlavorErrorCode::FLAVOR_ERROR_CODE_RESYNC_REQUIRED)
    }

    pub fn is_unavailable(&self) -> bool {
        self.flavor.is_none() && matches!(self.code, ErrorCode::Unavailable | ErrorCode::Unknown)
    }

    pub fn retryable(&self) -> bool {
        self.flavor.as_ref().map_or(self.is_unavailable(), |d| d.retryable)
    }
}

impl From<ConnectError> for IpcError {
    fn from(err: ConnectError) -> Self {
        let flavor = err
            .details
            .iter()
            .filter(|d| d.type_url.trim_start_matches("type.googleapis.com/") == "flavor.v1.FlavorErrorDetail")
            .filter_map(|d| d.value.as_deref())
            .filter_map(|v| {
                base64::engine::general_purpose::STANDARD_NO_PAD
                    .decode(v.trim_end_matches('='))
                    .ok()
            })
            .find_map(|b| FlavorErrorDetail::decode_from_slice(&b).ok());
        let message = flavor
            .as_ref()
            .map(|d| d.safe_message.clone())
            .unwrap_or_else(|| err.message.clone().unwrap_or_default());
        Self { code: err.code, flavor, message }
    }
}

impl fmt::Display for IpcError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}: {}", self.code.as_str(), self.message)
    }
}

impl std::error::Error for IpcError {}
