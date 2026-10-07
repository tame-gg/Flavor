const COMMANDS: &[&str] = &[
    "get_snapshot",
    "watch_events",
    "add_network",
    "update_network",
    "connect_network",
    "disconnect_network",
    "enroll_network",
    "remove_network",
    "delete_network_identity",
    "run_diagnostics",
    "inspect_destination",
    "list_conflicts",
    "create_workspace",
    "update_workspace",
    "delete_workspace",
    "activate_workspace",
    "deactivate_workspace",
    "set_destination_preference",
    "delete_destination_preference",
    "open_auth_url",
    "get_settings",
    "set_settings",
    "set_tray_summary",
    "app_version",
];

fn main() {
    tauri_build::try_build(
        tauri_build::Attributes::new()
            .app_manifest(tauri_build::AppManifest::new().commands(COMMANDS)),
    )
    .expect("tauri build");
}
