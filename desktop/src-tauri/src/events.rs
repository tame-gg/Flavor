use std::sync::Mutex;

use lattice_ipc::proto::{DaemonEvent, WatchEventsRequest};
use serde::Serialize;
use tauri::async_runtime::JoinHandle;
use tauri::{AppHandle, Emitter, State};

use crate::commands::{Daemon, Result, UiError};

pub const CHANNEL: &str = "lattice://daemon-event";

#[derive(Default)]
pub struct Watcher {
    current: Mutex<Option<JoinHandle<()>>>,
}

#[derive(Clone, Serialize)]
#[serde(tag = "kind", rename_all = "camelCase")]
enum Message {
    #[serde(rename_all = "camelCase")]
    Event { watch_id: u64, event: DaemonEvent },
    #[serde(rename_all = "camelCase")]
    Closed { watch_id: u64, error: Option<UiError> },
}

#[tauri::command]
pub async fn watch_events(
    app: AppHandle,
    daemon: State<'_, Daemon>,
    watcher: State<'_, Watcher>,
    watch_id: u64,
    instance_id: String,
    after_sequence: String,
) -> Result<()> {
    let after_sequence: u64 = after_sequence.parse().map_err(|_| UiError {
        kind: "invalid",
        code: None,
        message: "Invalid event sequence.".into(),
        retryable: false,
    })?;
    let client = daemon.client()?.clone();
    let mut slot = watcher.current.lock().unwrap();
    if let Some(task) = slot.take() {
        task.abort();
    }
    let task = tauri::async_runtime::spawn(async move {
        let request = WatchEventsRequest { daemon_instance_id: instance_id, after_sequence, ..Default::default() };
        let error = match client.events.watch_events(request).await {
            Err(e) => Some(UiError::from(e)),
            Ok(mut stream) => loop {
                match stream.message::<DaemonEvent>().await {
                    Ok(Some(msg)) => {
                        let _ = app.emit(CHANNEL, Message::Event { watch_id, event: msg.to_owned_message() });
                    }
                    Ok(None) => break None,
                    Err(e) => break Some(UiError::from(e)),
                }
            },
        };
        let _ = app.emit(CHANNEL, Message::Closed { watch_id, error });
    });
    *slot = Some(task);
    Ok(())
}
