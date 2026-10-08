mod commands;
mod events;
mod settings;
mod tray;

use tauri::{Manager, WindowEvent};

fn main() {
    tauri::Builder::default()
        .plugin(tauri_plugin_opener::init())
        .manage(commands::Daemon::new(flavor_ipc::default_socket_path()))
        .manage(events::Watcher::default())
        .manage(settings::Store::open(settings::default_path()))
        .setup(|app| Ok(tray::install(app)?))
        .on_window_event(|window, event| {
            if let WindowEvent::CloseRequested { api, .. } = event {
                let store = window.state::<settings::Store>();
                if store.get().settings.close_behavior == settings::CloseBehavior::Tray {
                    api.prevent_close();
                    let _ = window.hide();
                }
            }
        })
        .invoke_handler(tauri::generate_handler![
            commands::get_snapshot,
            commands::add_network,
            commands::update_network,
            commands::connect_network,
            commands::disconnect_network,
            commands::enroll_network,
            commands::remove_network,
            commands::delete_network_identity,
            commands::run_diagnostics,
            commands::inspect_destination,
            commands::describe_device,
            commands::list_conflicts,
            commands::create_workspace,
            commands::update_workspace,
            commands::delete_workspace,
            commands::activate_workspace,
            commands::deactivate_workspace,
            commands::set_destination_preference,
            commands::delete_destination_preference,
            commands::open_auth_url,
            commands::get_settings,
            commands::set_settings,
            commands::app_version,
            events::watch_events,
            tray::set_tray_summary,
        ])
        .build(tauri::generate_context!())
        .expect("build flavor desktop")
        .run(|_app, _event| {
            #[cfg(target_os = "macos")]
            if let tauri::RunEvent::Reopen { .. } = _event {
                tray::show_main(_app);
            }
        });
}
