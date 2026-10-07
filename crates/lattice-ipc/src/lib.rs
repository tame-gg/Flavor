use std::fmt;
use std::path::{Path, PathBuf};

use base64::Engine as _;
use buffa::Message as _;
use connectrpc::client::{ClientConfig, Http2Connection, SharedHttp2Connection};
use connectrpc::ErrorCode;
use lattice_proto::lattice::v1::{
    DaemonServiceClient, DeviceServiceClient, DiagnosticsServiceClient, EventServiceClient,
    LatticeErrorCode, LatticeErrorDetail, NetworkServiceClient,
};

pub use connectrpc::ConnectError;
pub use lattice_proto::lattice::v1 as proto;

pub const PROTOCOL_MAJOR: u32 = 1;

type Transport = SharedHttp2Connection;

#[derive(Clone)]
pub struct LatticeIpcClient {
    pub daemon: DaemonServiceClient<Transport>,
    pub networks: NetworkServiceClient<Transport>,
    pub devices: DeviceServiceClient<Transport>,
    pub diagnostics: DiagnosticsServiceClient<Transport>,
    pub events: EventServiceClient<Transport>,
}

impl LatticeIpcClient {
    pub fn new(socket: impl Into<PathBuf>) -> Self {
        let authority: http::Uri = "http://latticed".parse().expect("static uri");
        let transport = Http2Connection::lazy_unix(socket, authority.clone()).shared(256);
        let config = || ClientConfig::new(authority.clone());
        Self {
            daemon: DaemonServiceClient::new(transport.clone(), config()),
            networks: NetworkServiceClient::new(transport.clone(), config()),
            devices: DeviceServiceClient::new(transport.clone(), config()),
            diagnostics: DiagnosticsServiceClient::new(transport.clone(), config()),
            events: EventServiceClient::new(transport, config()),
        }
    }
}

pub fn default_socket_path() -> Option<PathBuf> {
    let runtime = std::env::var_os("XDG_RUNTIME_DIR")?;
    let runtime = Path::new(&runtime);
    runtime
        .is_absolute()
        .then(|| runtime.join("lattice").join("latticed.sock"))
}

#[derive(Debug, Clone)]
pub struct IpcError {
    pub code: ErrorCode,
    pub lattice: Option<LatticeErrorDetail>,
    pub message: String,
}

impl IpcError {
    pub fn lattice_code(&self) -> Option<LatticeErrorCode> {
        self.lattice.as_ref().and_then(|d| d.code.as_known())
    }

    pub fn is_resync_required(&self) -> bool {
        self.lattice_code() == Some(LatticeErrorCode::LATTICE_ERROR_CODE_RESYNC_REQUIRED)
    }

    pub fn is_unavailable(&self) -> bool {
        self.lattice.is_none() && matches!(self.code, ErrorCode::Unavailable | ErrorCode::Unknown)
    }

    pub fn retryable(&self) -> bool {
        self.lattice.as_ref().map_or(self.is_unavailable(), |d| d.retryable)
    }
}

impl From<ConnectError> for IpcError {
    fn from(err: ConnectError) -> Self {
        let lattice = err
            .details
            .iter()
            .filter(|d| d.type_url.trim_start_matches("type.googleapis.com/") == "lattice.v1.LatticeErrorDetail")
            .filter_map(|d| d.value.as_deref())
            .filter_map(|v| {
                base64::engine::general_purpose::STANDARD_NO_PAD
                    .decode(v.trim_end_matches('='))
                    .ok()
            })
            .find_map(|b| LatticeErrorDetail::decode_from_slice(&b).ok());
        let message = lattice
            .as_ref()
            .map(|d| d.safe_message.clone())
            .unwrap_or_else(|| err.message.clone().unwrap_or_default());
        Self { code: err.code, lattice, message }
    }
}

impl fmt::Display for IpcError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}: {}", self.code.as_str(), self.message)
    }
}

impl std::error::Error for IpcError {}
