use std::fs;
use std::io::Write;
use std::os::unix::fs::{DirBuilderExt, OpenOptionsExt};
use std::path::{Path, PathBuf};
use std::sync::Mutex;

use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum Theme {
    System,
    Light,
    Dark,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum CloseBehavior {
    Tray,
    QuitGui,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Settings {
    pub theme: Theme,
    pub close_behavior: CloseBehavior,
}

impl Default for Settings {
    fn default() -> Self {
        Self { theme: Theme::System, close_behavior: CloseBehavior::Tray }
    }
}

#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Loaded {
    #[serde(flatten)]
    pub settings: Settings,
    pub warning: Option<String>,
}

#[derive(Deserialize, Default)]
struct RawFile {
    desktop: Option<RawDesktop>,
    ui: Option<RawUi>,
}

#[derive(Deserialize, Default)]
struct RawDesktop {
    close_behavior: Option<String>,
}

#[derive(Deserialize, Default)]
struct RawUi {
    theme: Option<String>,
}

#[derive(Serialize)]
struct FileOut {
    version: u32,
    desktop: DesktopOut,
    ui: UiOut,
}

#[derive(Serialize)]
struct DesktopOut {
    close_behavior: CloseBehavior,
}

#[derive(Serialize)]
struct UiOut {
    theme: Theme,
}

pub fn default_path() -> Option<PathBuf> {
    let base = std::env::var_os("XDG_CONFIG_HOME")
        .map(PathBuf::from)
        .filter(|p| p.is_absolute())
        .or_else(flavor_ipc::macos_support_dir)
        .or_else(|| std::env::var_os("HOME").map(|h| PathBuf::from(h).join(".config")))?;
    base.is_absolute().then(|| base.join("flavor").join("config.toml"))
}

pub fn load(path: &Path) -> Loaded {
    let text = match fs::read_to_string(path) {
        Ok(t) => t,
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => {
            return Loaded { settings: Settings::default(), warning: None };
        }
        Err(_) => return fallback("Settings file could not be read; using defaults."),
    };
    let raw: RawFile = match toml::from_str(&text) {
        Ok(r) => r,
        Err(_) => return fallback("Settings file is malformed; using defaults. It will be replaced only when you change a setting."),
    };
    let mut warning = None;
    let theme = match raw.ui.and_then(|u| u.theme).as_deref() {
        None | Some("system") => Theme::System,
        Some("light") => Theme::Light,
        Some("dark") => Theme::Dark,
        Some(_) => {
            warning = Some("Unknown theme in settings file; using system theme.".to_string());
            Theme::System
        }
    };
    let close_behavior = match raw.desktop.and_then(|d| d.close_behavior).as_deref() {
        None | Some("tray") => CloseBehavior::Tray,
        Some("quit_gui") => CloseBehavior::QuitGui,
        Some(_) => {
            warning = Some("Unknown close behavior in settings file; keeping Flavor in the tray.".to_string());
            CloseBehavior::Tray
        }
    };
    Loaded { settings: Settings { theme, close_behavior }, warning }
}

fn fallback(msg: &str) -> Loaded {
    Loaded { settings: Settings::default(), warning: Some(msg.to_string()) }
}

pub fn save(path: &Path, settings: Settings) -> std::io::Result<()> {
    let dir = path.parent().ok_or_else(|| std::io::Error::other("settings path has no parent"))?;
    fs::DirBuilder::new().recursive(true).mode(0o700).create(dir)?;
    let body = toml::to_string(&FileOut {
        version: 1,
        desktop: DesktopOut { close_behavior: settings.close_behavior },
        ui: UiOut { theme: settings.theme },
    })
    .map_err(std::io::Error::other)?;
    let tmp = dir.join(format!(".config.toml.{}.tmp", std::process::id()));
    let result = (|| {
        let mut f = fs::OpenOptions::new().write(true).create_new(true).mode(0o600).open(&tmp)?;
        f.write_all(body.as_bytes())?;
        f.sync_all()?;
        fs::rename(&tmp, path)?;
        fs::File::open(dir)?.sync_all()
    })();
    if result.is_err() {
        let _ = fs::remove_file(&tmp);
    }
    result
}

pub struct Store {
    path: Option<PathBuf>,
    current: Mutex<Loaded>,
}

impl Store {
    pub fn open(path: Option<PathBuf>) -> Self {
        let loaded = match &path {
            Some(p) => load(p),
            None => fallback("No configuration directory is available; settings will not be saved."),
        };
        Self { path, current: Mutex::new(loaded) }
    }

    pub fn get(&self) -> Loaded {
        self.current.lock().unwrap().clone()
    }

    pub fn set(&self, settings: Settings) -> std::io::Result<Loaded> {
        let path = self.path.as_ref().ok_or_else(|| std::io::Error::other("no configuration directory"))?;
        save(path, settings)?;
        let loaded = Loaded { settings, warning: None };
        *self.current.lock().unwrap() = loaded.clone();
        Ok(loaded)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::os::unix::fs::PermissionsExt;

    #[test]
    fn missing_file_gives_defaults_without_warning() {
        let dir = tempfile::tempdir().unwrap();
        let l = load(&dir.path().join("config.toml"));
        assert_eq!(l.settings, Settings::default());
        assert!(l.warning.is_none());
    }

    #[test]
    fn round_trip_is_private_and_matches_schema() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("flavor/config.toml");
        let s = Settings { theme: Theme::Dark, close_behavior: CloseBehavior::QuitGui };
        save(&path, s).unwrap();
        assert_eq!(load(&path).settings, s);
        let text = fs::read_to_string(&path).unwrap();
        assert!(text.contains("version = 1"));
        assert!(text.contains("close_behavior = \"quit_gui\""));
        assert!(text.contains("theme = \"dark\""));
        assert_eq!(fs::metadata(&path).unwrap().permissions().mode() & 0o777, 0o600);
        assert_eq!(fs::metadata(path.parent().unwrap()).unwrap().permissions().mode() & 0o777, 0o700);
    }

    #[test]
    fn malformed_file_is_never_overwritten_by_loading() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("config.toml");
        fs::write(&path, "this is = = not toml").unwrap();
        let store = Store::open(Some(path.clone()));
        assert!(store.get().warning.is_some());
        assert_eq!(store.get().settings, Settings::default());
        assert_eq!(fs::read_to_string(&path).unwrap(), "this is = = not toml");
    }

    #[test]
    fn unknown_enum_values_fall_back_with_warning() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("config.toml");
        fs::write(&path, "version = 1\n[ui]\ntheme = \"neon\"\n[desktop]\nclose_behavior = \"tray\"\n").unwrap();
        let l = load(&path);
        assert_eq!(l.settings.theme, Theme::System);
        assert!(l.warning.is_some());
    }
}
