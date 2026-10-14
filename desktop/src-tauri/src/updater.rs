use std::sync::Mutex;
use std::time::Duration;

use serde::Serialize;
use serde_json::Value;
use tauri::{AppHandle, Emitter, Manager};
use tauri_plugin_updater::{Update, UpdaterExt};

use crate::commands::{Daemon, Result, UiError};

pub const CHANNEL: &str = "flavor://update";
const EVERY: Duration = Duration::from_secs(6 * 3600);

/// The newest update found by the background check, waiting for the user to accept it.
#[derive(Default)]
pub struct Pending(Mutex<Option<Update>>);

#[derive(Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Offer {
    version: String,
    flavord_update: Option<String>,
}

async fn offer(app: &AppHandle, update: &Update) -> Offer {
    Offer {
        version: update.version.clone(),
        flavord_update: flavord_update(app, update).await,
    }
}

async fn flavord_update(app: &AppHandle, update: &Update) -> Option<String> {
    if !cfg!(target_os = "macos") {
        return None;
    }
    let running = daemon_protocol(app)
        .await
        .unwrap_or(flavor_ipc::PROTOCOL_MAJOR);
    needs_flavord(&update.raw_json, running).then(|| flavord_instructions(&update.version))
}

async fn daemon_protocol(app: &AppHandle) -> Option<u32> {
    let daemon = app.state::<Daemon>();
    let info = daemon
        .client()
        .ok()?
        .daemon
        .get_daemon_info(Default::default())
        .await
        .ok()?;
    Some(info.into_owned().protocol_major)
}

fn needs_flavord(manifest: &Value, running: u32) -> bool {
    manifest
        .get("protocol_major")
        .and_then(Value::as_u64)
        .is_some_and(|p| p != u64::from(running))
}

fn flavord_instructions(version: &str) -> String {
    format!(
        "Flavor {version} needs flavord {version}. Update it first with brew upgrade flavor && brew services restart flavor, \
         or install the {version} tarball from https://github.com/tame-gg/Flavor/releases."
    )
}

async fn check(app: &AppHandle) -> tauri_plugin_updater::Result<()> {
    let Some(update) = app.updater()?.check().await? else {
        return Ok(());
    };
    let offer = offer(app, &update).await;
    *app.state::<Pending>().0.lock().unwrap() = Some(update);
    let _ = app.emit(CHANNEL, offer);
    Ok(())
}

/// Checks on launch and every six hours after, in release builds on macOS and Windows only.
/// A dev build is always older than what the CDN serves, and Linux installs come from the
/// tarball or a package manager, which also update flavord and its system files.
pub fn spawn(app: AppHandle) {
    if cfg!(debug_assertions) || cfg!(target_os = "linux") {
        return;
    }
    std::thread::spawn(move || loop {
        if let Err(e) = tauri::async_runtime::block_on(check(&app)) {
            eprintln!("update check failed: {e}");
        }
        std::thread::sleep(EVERY);
    });
}

#[tauri::command]
pub async fn update_pending(app: AppHandle) -> Option<Offer> {
    let update = app.state::<Pending>().0.lock().unwrap().clone()?;
    Some(offer(&app, &update).await)
}

#[tauri::command]
pub async fn update_install(app: AppHandle) -> Result<()> {
    let update = app.state::<Pending>().0.lock().unwrap().clone();
    let update = update.ok_or_else(|| failed("No update is waiting to be installed."))?;
    if let Some(message) = flavord_update(&app, &update).await {
        return Err(failed(&message));
    }
    update
        .download_and_install(|_, _| {}, || {})
        .await
        .map_err(|e| failed(&format!("The update could not be installed: {e}")))?;
    app.restart();
}

fn failed(message: &str) -> UiError {
    UiError {
        kind: "invalid",
        code: None,
        message: message.into(),
        retryable: false,
    }
}

#[cfg(test)]
mod tests {
    use super::needs_flavord;
    use serde_json::json;

    #[test]
    fn a_different_protocol_needs_flavord_updated_first() {
        assert!(needs_flavord(&json!({"protocol_major": 2}), 1));
        assert!(!needs_flavord(&json!({"protocol_major": 1}), 1));
        assert!(!needs_flavord(&json!({"version": "0.2.0"}), 1));
    }
}
