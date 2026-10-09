use tauri::image::Image;
use tauri::menu::{Menu, MenuItem, PredefinedMenuItem};
use tauri::tray::TrayIconBuilder;
use tauri::{App, AppHandle, Manager, State, Wry};

#[cfg(windows)]
const TRAY_ICON: &[u8] = include_bytes!("../icons/32x32.png");
#[cfg(not(windows))]
const TRAY_ICON: &[u8] = include_bytes!("../icons/tray.png");

pub struct Status(MenuItem<Wry>);

pub fn install(app: &App) -> tauri::Result<()> {
    let show = MenuItem::with_id(app, "show", "Show Flavor", true, None::<&str>)?;
    let status = MenuItem::with_id(app, "status", "Connecting to daemon…", false, None::<&str>)?;
    let quit = MenuItem::with_id(app, "quit", "Quit Flavor", true, None::<&str>)?;
    let menu = Menu::with_items(
        app,
        &[&show, &PredefinedMenuItem::separator(app)?, &status, &PredefinedMenuItem::separator(app)?, &quit],
    )?;
    TrayIconBuilder::with_id("main")
        .icon(Image::from_bytes(TRAY_ICON)?)
        .icon_as_template(true)
        .tooltip("Flavor")
        .menu(&menu)
        .on_menu_event(|app, event| match event.id().as_ref() {
            "show" => show_main(app),
            "quit" => app.exit(0),
            _ => {}
        })
        .build(app)?;
    app.manage(Status(status));
    Ok(())
}

pub fn show_main(app: &AppHandle) {
    if let Some(w) = app.get_webview_window("main") {
        let _ = w.show();
        let _ = w.unminimize();
        let _ = w.set_focus();
    }
}

#[tauri::command]
pub fn set_tray_summary(status: State<'_, Status>, text: String) {
    let text: String = text.chars().filter(|c| !c.is_control()).take(80).collect();
    let _ = status.0.set_text(text);
}
