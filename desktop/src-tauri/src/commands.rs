use std::path::PathBuf;
use std::sync::OnceLock;

use buffa::Enumeration;
use flavor_ipc::proto::*;
use flavor_ipc::{ConnectError, IpcError, FlavorIpcClient};
use serde::Serialize;
use tauri::{AppHandle, State};
use tauri_plugin_opener::OpenerExt;

use crate::settings::{self, Settings};

pub struct Daemon {
    socket: Option<PathBuf>,
    client: OnceLock<FlavorIpcClient>,
}

impl Daemon {
    pub fn new(socket: Option<PathBuf>) -> Self {
        Self { socket, client: OnceLock::new() }
    }

    pub fn client(&self) -> Result<&FlavorIpcClient> {
        let socket = self.socket.as_ref().ok_or_else(|| UiError {
            kind: "unavailable",
            code: None,
            message: "XDG_RUNTIME_DIR is not set, so the daemon socket cannot be located.".into(),
            retryable: false,
        })?;
        Ok(self.client.get_or_init(|| FlavorIpcClient::new(socket)))
    }
}

#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct UiError {
    pub kind: &'static str,
    pub code: Option<&'static str>,
    pub message: String,
    pub retryable: bool,
}

impl UiError {
    fn invalid(message: &str) -> Self {
        Self { kind: "invalid", code: None, message: message.into(), retryable: false }
    }
}

impl From<IpcError> for UiError {
    fn from(e: IpcError) -> Self {
        let kind = match () {
            _ if e.flavor.is_some() => "daemon",
            _ if e.is_unavailable() => "unavailable",
            _ => "transport",
        };
        Self { kind, code: e.flavor_code().map(|c| c.proto_name()), retryable: e.retryable(), message: e.message }
    }
}

impl From<ConnectError> for UiError {
    fn from(e: ConnectError) -> Self {
        IpcError::from(e).into()
    }
}

pub type Result<T> = std::result::Result<T, UiError>;

#[tauri::command]
pub async fn get_snapshot(daemon: State<'_, Daemon>) -> Result<GetStateSnapshotResponse> {
    Ok(daemon.client()?.daemon.get_state_snapshot(Default::default()).await?.into_owned())
}

#[tauri::command]
pub async fn add_network(
    daemon: State<'_, Daemon>,
    display_name: String,
    provider: String,
    control_url: String,
    auto_connect: bool,
) -> Result<Network> {
    let provider = match provider.as_str() {
        "tailscale" => ProviderType::PROVIDER_TYPE_TAILSCALE,
        "headscale" => ProviderType::PROVIDER_TYPE_HEADSCALE,
        _ => return Err(UiError::invalid("Unsupported provider.")),
    };
    let res = daemon
        .client()?
        .networks
        .add_network(AddNetworkRequest {
            display_name,
            provider: provider.into(),
            control_url,
            auto_connect,
            ..Default::default()
        })
        .await?
        .into_owned();
    Ok(res.network.into_option().unwrap_or_default())
}

#[tauri::command]
pub async fn update_network(
    daemon: State<'_, Daemon>,
    network_id: String,
    display_name: Option<String>,
    auto_connect: Option<bool>,
) -> Result<Network> {
    let res = daemon
        .client()?
        .networks
        .update_network(UpdateNetworkRequest { network_id, display_name, auto_connect, ..Default::default() })
        .await?
        .into_owned();
    Ok(res.network.into_option().unwrap_or_default())
}

#[tauri::command]
pub async fn connect_network(daemon: State<'_, Daemon>, network_id: String) -> Result<()> {
    daemon.client()?.networks.connect_network(ConnectNetworkRequest { network_id, ..Default::default() }).await?;
    Ok(())
}

#[tauri::command]
pub async fn disconnect_network(daemon: State<'_, Daemon>, network_id: String) -> Result<()> {
    daemon.client()?.networks.disconnect_network(DisconnectNetworkRequest { network_id, ..Default::default() }).await?;
    Ok(())
}

#[tauri::command]
pub async fn enroll_network(daemon: State<'_, Daemon>, network_id: String, pre_auth_key: String) -> Result<()> {
    let enrollment = EnrollmentCredential {
        credential: Some(enrollment_credential::Credential::PreAuthKey(pre_auth_key)),
        ..Default::default()
    };
    daemon
        .client()?
        .networks
        .enroll_network(EnrollNetworkRequest { network_id, enrollment: enrollment.into(), ..Default::default() })
        .await?;
    Ok(())
}

#[tauri::command]
pub async fn remove_network(daemon: State<'_, Daemon>, network_id: String) -> Result<()> {
    daemon.client()?.networks.remove_network(RemoveNetworkRequest { network_id, ..Default::default() }).await?;
    Ok(())
}

#[tauri::command]
pub async fn delete_network_identity(daemon: State<'_, Daemon>, network_id: String) -> Result<()> {
    daemon
        .client()?
        .networks
        .delete_network_identity(DeleteNetworkIdentityRequest { network_id, ..Default::default() })
        .await?;
    Ok(())
}

#[tauri::command]
pub async fn run_diagnostics(daemon: State<'_, Daemon>) -> Result<RunDiagnosticsResponse> {
    Ok(daemon.client()?.diagnostics.run_diagnostics(Default::default()).await?.into_owned())
}

#[tauri::command]
pub async fn inspect_destination(daemon: State<'_, Daemon>, destination: String) -> Result<InspectDestinationResponse> {
    Ok(daemon
        .client()?
        .inspector
        .inspect_destination(InspectDestinationRequest { destination, ..Default::default() })
        .await?
        .into_owned())
}

#[tauri::command]
pub async fn describe_device(daemon: State<'_, Daemon>, network_id: String, node_id: String) -> Result<DescribeDeviceResponse> {
    Ok(daemon
        .client()?
        .inspector
        .describe_device(DescribeDeviceRequest { network_id, node_id, ..Default::default() })
        .await?
        .into_owned())
}

#[tauri::command]
pub async fn list_conflicts(daemon: State<'_, Daemon>) -> Result<ListConflictsResponse> {
    Ok(daemon.client()?.conflicts.list_conflicts(Default::default()).await?.into_owned())
}

#[tauri::command]
pub async fn create_workspace(
    daemon: State<'_, Daemon>,
    name: String,
    description: String,
    network_ids: Vec<String>,
) -> Result<Workspace> {
    let res = daemon
        .client()?
        .workspaces
        .create_workspace(CreateWorkspaceRequest { name, description, network_ids, ..Default::default() })
        .await?
        .into_owned();
    Ok(res.workspace.into_option().unwrap_or_default())
}

#[tauri::command]
pub async fn update_workspace(
    daemon: State<'_, Daemon>,
    workspace_id: String,
    name: Option<String>,
    description: Option<String>,
    network_ids: Option<Vec<String>>,
) -> Result<Workspace> {
    let networks = network_ids.map(|network_ids| NetworkIDList { network_ids, ..Default::default() });
    let res = daemon
        .client()?
        .workspaces
        .update_workspace(UpdateWorkspaceRequest {
            workspace_id,
            name,
            description,
            networks: networks.into(),
            ..Default::default()
        })
        .await?
        .into_owned();
    Ok(res.workspace.into_option().unwrap_or_default())
}

#[tauri::command]
pub async fn delete_workspace(daemon: State<'_, Daemon>, workspace_id: String) -> Result<()> {
    daemon
        .client()?
        .workspaces
        .delete_workspace(DeleteWorkspaceRequest { workspace_id, ..Default::default() })
        .await?;
    Ok(())
}

#[tauri::command]
pub async fn activate_workspace(
    daemon: State<'_, Daemon>,
    workspace_id: String,
    disconnect_others: bool,
) -> Result<ActivateWorkspaceResponse> {
    Ok(daemon
        .client()?
        .workspaces
        .activate_workspace(ActivateWorkspaceRequest { workspace_id, disconnect_others, ..Default::default() })
        .await?
        .into_owned())
}

#[tauri::command]
pub async fn deactivate_workspace(daemon: State<'_, Daemon>) -> Result<()> {
    daemon.client()?.workspaces.deactivate_workspace(Default::default()).await?;
    Ok(())
}

#[tauri::command]
pub async fn set_destination_preference(
    daemon: State<'_, Daemon>,
    destination: String,
    network_id: String,
) -> Result<DestinationPreference> {
    let res = daemon
        .client()?
        .preferences
        .set_destination_preference(SetDestinationPreferenceRequest { destination, network_id, ..Default::default() })
        .await?
        .into_owned();
    Ok(res.preference.into_option().unwrap_or_default())
}

#[tauri::command]
pub async fn delete_destination_preference(daemon: State<'_, Daemon>, destination: String) -> Result<()> {
    daemon
        .client()?
        .preferences
        .delete_destination_preference(DeleteDestinationPreferenceRequest { destination, ..Default::default() })
        .await?;
    Ok(())
}

#[tauri::command]
pub async fn open_auth_url(
    app: AppHandle,
    daemon: State<'_, Daemon>,
    network_id: String,
    flow_id: String,
) -> Result<()> {
    let res = daemon
        .client()?
        .networks
        .get_network(GetNetworkRequest { network_id, ..Default::default() })
        .await?
        .into_owned();
    let prompt = res.network.authentication.clone();
    if prompt.flow_id.is_empty() || prompt.flow_id != flow_id {
        return Err(UiError::invalid("This sign-in link has expired. Use the current link instead."));
    }
    let url = safe_auth_url(&prompt.auth_url).ok_or_else(|| UiError::invalid("The sign-in link is not a web address."))?;
    app.opener()
        .open_url(url.as_str(), None::<&str>)
        .map_err(|_| UiError::invalid("No browser could be opened for the sign-in link."))
}

pub fn safe_auth_url(raw: &str) -> Option<tauri::Url> {
    let url = tauri::Url::parse(raw).ok()?;
    let ok = matches!(url.scheme(), "http" | "https")
        && url.host_str().is_some_and(|h| !h.is_empty())
        && url.username().is_empty()
        && url.password().is_none();
    ok.then_some(url)
}

#[tauri::command]
pub fn get_settings(store: State<'_, settings::Store>) -> settings::Loaded {
    store.get()
}

#[tauri::command]
pub fn set_settings(store: State<'_, settings::Store>, settings: Settings) -> Result<settings::Loaded> {
    store.set(settings).map_err(|_| UiError::invalid("Settings could not be saved."))
}

#[tauri::command]
pub fn app_version() -> &'static str {
    env!("CARGO_PKG_VERSION")
}

#[cfg(test)]
mod tests {
    use super::safe_auth_url;

    #[test]
    fn auth_urls_must_be_plain_web_links() {
        assert!(safe_auth_url("https://login.tailscale.com/a/abc").is_some());
        assert!(safe_auth_url("http://127.0.0.1:8080/register/xyz").is_some());
        assert!(safe_auth_url("https://idp.example.org/oidc?state=1").is_some());
        for bad in [
            "javascript:alert(1)",
            "file:///etc/passwd",
            "data:text/html,hi",
            "https://user:pass@example.com/",
            "ftp://example.com/",
            "/relative/path",
            "",
        ] {
            assert!(safe_auth_url(bad).is_none(), "{bad}");
        }
    }
}
