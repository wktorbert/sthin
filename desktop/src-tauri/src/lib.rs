//! Sthin desktop: a window plus a Tray over the bundled `sthin serve` sidecar.
//! Rust's job is process ownership and window lifecycle; the Tray menu and all
//! device logic live in the frontend and in Go.

mod sidecar;
mod wire;

use serde_json::Value;
use sidecar::{Sidecar, Status};
use tauri::image::Image;
use tauri::tray::TrayIconBuilder;
use tauri::{AppHandle, Manager, RunEvent, State, WindowEvent};

/// The one generic call into Serve. Method names and payloads are Serve's.
#[tauri::command]
async fn serve_call(
    state: State<'_, Sidecar>,
    id: u64,
    method: String,
    params: Value,
) -> Result<Value, Value> {
    state.call(id, &method, params).await
}

#[tauri::command]
fn serve_status(state: State<'_, Sidecar>) -> Status {
    state.status()
}

#[tauri::command]
fn serve_retry(app: AppHandle) {
    Sidecar::retry(&app);
}

/// Quit for real (the window's close button only hides it).
#[tauri::command]
fn quit_app(app: AppHandle) {
    app.state::<Sidecar>().kill();
    app.exit(0);
}

#[tauri::command]
fn show_window(app: AppHandle) {
    show_main(&app);
}

fn show_main(app: &AppHandle) {
    if let Some(w) = app.get_webview_window("main") {
        let _ = w.unminimize();
        let _ = w.show();
        let _ = w.set_focus();
    }
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    let app = tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_notification::init())
        .plugin(tauri_plugin_opener::init())
        .manage(Sidecar::new())
        .invoke_handler(tauri::generate_handler![
            serve_call,
            serve_status,
            serve_retry,
            quit_app,
            show_window
        ])
        .setup(|app| {
            // The Tray starts with no menu; the frontend fills it from the
            // device list (src/tray.ts) and rebuilds it on every change.
            let icon = Image::from_bytes(include_bytes!("../icons/tray.png"))?;
            TrayIconBuilder::with_id("main")
                .icon(icon)
                .icon_as_template(true)
                .tooltip("Sthin")
                .show_menu_on_left_click(true)
                .build(app)?;
            Sidecar::start(app.handle());
            Ok(())
        })
        .on_window_event(|window, event| {
            // Closing the window hides it; Sthin keeps running in the Tray.
            if let WindowEvent::CloseRequested { api, .. } = event {
                if window.label() == "main" {
                    api.prevent_close();
                    let _ = window.hide();
                }
            }
        })
        .build(tauri::generate_context!())
        .expect("error while building the Sthin desktop app");

    app.run(|app, event| match event {
        // macOS: clicking the Dock icon brings the hidden window back.
        #[cfg(target_os = "macos")]
        RunEvent::Reopen { .. } => show_main(app),
        RunEvent::Exit => app.state::<Sidecar>().kill(),
        _ => {}
    });
}
