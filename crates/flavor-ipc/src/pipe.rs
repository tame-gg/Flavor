use std::io;
use std::os::windows::io::AsRawHandle;
use std::path::{Path, PathBuf};
use std::time::Duration;

use hyper_util::rt::TokioIo;
use sha2::{Digest, Sha256};
use tokio::net::windows::named_pipe::{ClientOptions, NamedPipeClient};
use windows_sys::Win32::Foundation::{CloseHandle, ERROR_PIPE_BUSY, HANDLE};
use windows_sys::Win32::Security::{EqualSid, GetTokenInformation, TokenUser, TOKEN_QUERY, TOKEN_USER};
use windows_sys::Win32::System::Pipes::GetNamedPipeServerProcessId;
use windows_sys::Win32::System::Threading::{GetCurrentProcess, OpenProcess, OpenProcessToken, PROCESS_QUERY_LIMITED_INFORMATION};

pub(crate) fn name(dir: &Path) -> PathBuf {
    let sum = Sha256::digest(dir.to_string_lossy().as_bytes());
    let hex: String = sum[..16].iter().map(|b| format!("{b:02x}")).collect();
    PathBuf::from(format!(r"\\.\pipe\flavor-{hex}"))
}

pub(crate) fn connector(
    path: PathBuf,
) -> impl tower::Service<http::Uri, Response = TokioIo<NamedPipeClient>, Error = io::Error, Future: Send + 'static> + Send + 'static {
    tower::service_fn(move |_: http::Uri| {
        let path = path.clone();
        async move { open(&path).await.map(TokioIo::new) }
    })
}

async fn open(path: &Path) -> io::Result<NamedPipeClient> {
    let client = loop {
        match ClientOptions::new().open(path) {
            Ok(client) => break client,
            Err(e) if e.raw_os_error() == Some(ERROR_PIPE_BUSY as i32) => tokio::time::sleep(Duration::from_millis(50)).await,
            Err(e) => return Err(e),
        }
    };
    if !server_is_current_user(client.as_raw_handle() as HANDLE) {
        return Err(io::Error::new(io::ErrorKind::PermissionDenied, "the flavord pipe belongs to another user"));
    }
    Ok(client)
}

fn server_is_current_user(pipe: HANDLE) -> bool {
    let mut pid = 0u32;
    if unsafe { GetNamedPipeServerProcessId(pipe, &mut pid) } == 0 {
        return false;
    }
    let server = unsafe { OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, 0, pid) };
    if server.is_null() {
        return false;
    }
    let theirs = token_user(server);
    unsafe { CloseHandle(server) };
    let ours = token_user(unsafe { GetCurrentProcess() });
    match (theirs, ours) {
        (Some(a), Some(b)) => unsafe { EqualSid(sid(&a), sid(&b)) != 0 },
        _ => false,
    }
}

fn token_user(process: HANDLE) -> Option<Vec<u64>> {
    let mut token: HANDLE = std::ptr::null_mut();
    if unsafe { OpenProcessToken(process, TOKEN_QUERY, &mut token) } == 0 {
        return None;
    }
    let mut len = 0u32;
    unsafe { GetTokenInformation(token, TokenUser, std::ptr::null_mut(), 0, &mut len) };
    let mut buf = vec![0u64; (len as usize).div_ceil(8)];
    let ok = unsafe { GetTokenInformation(token, TokenUser, buf.as_mut_ptr().cast(), len, &mut len) } != 0;
    unsafe { CloseHandle(token) };
    ok.then_some(buf)
}

fn sid(buf: &[u64]) -> *mut core::ffi::c_void {
    unsafe { (*(buf.as_ptr() as *const TOKEN_USER)).User.Sid }
}

#[cfg(test)]
mod tests {
    #[test]
    fn pipe_name_matches_the_daemon() {
        assert_eq!(
            super::name(std::path::Path::new(r"C:\Users\a\AppData\Local\flavor")),
            std::path::PathBuf::from(r"\\.\pipe\flavor-71bc6c60eb8a6c5e9d15707bbc888c1a")
        );
    }
}
