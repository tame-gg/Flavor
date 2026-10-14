use std::sync::Mutex;
use std::time::Duration;

use tauri::{AppHandle, Emitter, Manager, State};
use tauri_plugin_updater::{Update, UpdaterExt};

use crate::commands::{Result, UiError};

pub const CHANNEL: &str = "flavor://update";
const EVERY: Duration = Duration::from_secs(6 * 3600);

/// The newest update found by the background check, waiting for the user to accept it.
#[derive(Default)]
pub struct Pending(Mutex<Option<Update>>);

async fn check(app: &AppHandle) -> tauri_plugin_updater::Result<()> {
    let Some(update) = app.updater()?.check().await? else {
        return Ok(());
    };
    let version = update.version.clone();
    *app.state::<Pending>().0.lock().unwrap() = Some(update);
    let _ = app.emit(CHANNEL, version);
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
        if !app.state::<crate::settings::Store>().get().settings.update_checks {
            std::thread::sleep(EVERY);
            continue;
        }
        if let Err(e) = tauri::async_runtime::block_on(check(&app)) {
            eprintln!("update check failed: {e}");
        }
        std::thread::sleep(EVERY);
    });
}

#[tauri::command]
pub fn update_pending(pending: State<'_, Pending>) -> Option<String> {
    pending
        .0
        .lock()
        .unwrap()
        .as_ref()
        .map(|u| u.version.clone())
}

#[tauri::command]
pub async fn update_install(app: AppHandle) -> Result<()> {
    let update = app.state::<Pending>().0.lock().unwrap().clone();
    let update = update.ok_or_else(|| failed("No update is waiting to be installed."))?;
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
