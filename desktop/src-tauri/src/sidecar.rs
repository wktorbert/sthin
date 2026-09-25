//! Owns the `lean serve` child process: spawns it, frames requests, routes
//! responses to waiting calls and notifications to the webview, and respawns
//! it within a budget when it exits. It knows nothing about devices; the
//! domain lives in Go (Serve) and TypeScript (the frontend).

use crate::wire::{self, Incoming, RestartBudget, Tail};
use serde::Serialize;
use serde_json::{json, Value};
use std::collections::HashMap;
use std::sync::Mutex;
use std::time::{Duration, Instant};
use tauri::{AppHandle, Emitter, Manager};
use tauri_plugin_shell::process::{CommandChild, CommandEvent};
use tauri_plugin_shell::ShellExt;
use tokio::sync::oneshot;

type Reply = oneshot::Sender<Result<Value, Value>>;

/// What the frontend sees about the sidecar.
#[derive(Clone, Serialize)]
#[serde(tag = "state", rename_all = "snake_case")]
pub enum Status {
    Starting,
    Running { generation: u64 },
    Down { reason: String, stderr: Vec<String> },
}

struct Inner {
    child: Option<CommandChild>,
    /// Bumped on every spawn so events from a dead child are ignored.
    generation: u64,
    budget: RestartBudget,
    stderr: Tail,
    /// Set when the app stops the sidecar on purpose: its exit is not a crash.
    stopping: bool,
    status: Status,
}

pub struct Sidecar {
    inner: Mutex<Inner>,
    pending: Mutex<HashMap<u64, Reply>>,
}

impl Sidecar {
    pub fn new() -> Self {
        Self {
            inner: Mutex::new(Inner {
                child: None,
                generation: 0,
                budget: RestartBudget::new(3, Duration::from_secs(60)),
                stopping: false,
                stderr: Tail::new(50),
                status: Status::Starting,
            }),
            pending: Mutex::new(HashMap::new()),
        }
    }

    pub fn status(&self) -> Status {
        self.inner.lock().unwrap().status.clone()
    }

    /// Starts the sidecar. Called at launch and by the user's Retry.
    pub fn start(app: &AppHandle) {
        let state = app.state::<Sidecar>();
        let spawned = app
            .shell()
            .sidecar("lean")
            .map(|c| c.args(["serve"]))
            .and_then(|c| c.spawn());
        let mut inner = state.inner.lock().unwrap();
        match spawned {
            Ok((rx, child)) => {
                inner.generation += 1;
                inner.child = Some(child);
                inner.status = Status::Running {
                    generation: inner.generation,
                };
                let generation = inner.generation;
                drop(inner);
                let _ = app.emit("serve_status", state.status());
                let handle = app.clone();
                tauri::async_runtime::spawn(async move { pump(handle, rx, generation).await });
            }
            Err(e) => {
                inner
                    .stderr
                    .push(format!("could not start lean serve: {e}"));
                inner.status = Status::Down {
                    reason: format!("could not start lean serve: {e}"),
                    stderr: inner.stderr.lines(),
                };
                drop(inner);
                let _ = app.emit("serve_status", state.status());
            }
        }
    }

    /// The user asked to try again after a crash loop.
    pub fn retry(app: &AppHandle) {
        let state = app.state::<Sidecar>();
        {
            let mut inner = state.inner.lock().unwrap();
            if matches!(inner.status, Status::Running { .. }) {
                return;
            }
            inner.budget.reset();
            inner.status = Status::Starting;
        }
        Self::start(app);
        // The frontend reconnects (handshake, fresh subscriptions) on this.
        if matches!(state.status(), Status::Running { .. }) {
            let _ = app.emit("serve_restarted", ());
        }
    }

    /// Sends one request and waits for its response. `id` is chosen by the
    /// frontend so it can match `progress` notifications and `cancel` to it.
    pub async fn call(&self, id: u64, method: &str, params: Value) -> Result<Value, Value> {
        let (tx, rx) = oneshot::channel();
        {
            let mut pending = self.pending.lock().unwrap();
            if pending.contains_key(&id) {
                return Err(wire::error(
                    wire::CODE_DUPLICATE_ID,
                    "a request with this id is already in flight",
                ));
            }
            pending.insert(id, tx);
        }
        let written = {
            let mut inner = self.inner.lock().unwrap();
            match inner.child.as_mut() {
                Some(child) => child
                    .write(wire::request_line(id, method, &params).as_bytes())
                    .map_err(|e| e.to_string()),
                None => Err("lean serve is not running".to_string()),
            }
        };
        if let Err(msg) = written {
            self.pending.lock().unwrap().remove(&id);
            return Err(wire::error(wire::CODE_SERVE_DOWN, &msg));
        }
        rx.await
            .unwrap_or_else(|_| Err(wire::error(wire::CODE_SERVE_EXITED, "lean serve exited")))
    }

    /// Fails every waiting call: its answer died with the process.
    fn fail_pending(&self) {
        for (_, tx) in self.pending.lock().unwrap().drain() {
            let _ = tx.send(Err(wire::error(
                wire::CODE_SERVE_EXITED,
                "lean serve exited while this request was in flight",
            )));
        }
    }

    pub fn kill(&self) {
        let mut inner = self.inner.lock().unwrap();
        inner.stopping = true;
        if let Some(child) = inner.child.take() {
            let _ = child.kill();
        }
    }
}

async fn pump(
    app: AppHandle,
    mut rx: tauri::async_runtime::Receiver<CommandEvent>,
    generation: u64,
) {
    let state = app.state::<Sidecar>();
    while let Some(event) = rx.recv().await {
        match event {
            CommandEvent::Stdout(bytes) => {
                match wire::classify(String::from_utf8_lossy(&bytes).trim_end()) {
                    Incoming::Response { id, outcome } => {
                        if let Some(tx) = state.pending.lock().unwrap().remove(&id) {
                            let _ = tx.send(outcome);
                        }
                    }
                    Incoming::Notification { method, params } => {
                        let _ = app.emit(
                            "serve_notification",
                            json!({"method": method, "params": params}),
                        );
                    }
                    Incoming::Garbage(line) => state
                        .inner
                        .lock()
                        .unwrap()
                        .stderr
                        .push(format!("stdout: {line}")),
                }
            }
            CommandEvent::Stderr(bytes) => {
                state
                    .inner
                    .lock()
                    .unwrap()
                    .stderr
                    .push(String::from_utf8_lossy(&bytes).trim_end().to_string());
            }
            CommandEvent::Error(e) => state
                .inner
                .lock()
                .unwrap()
                .stderr
                .push(format!("error: {e}")),
            CommandEvent::Terminated(payload) => {
                on_exit(
                    &app,
                    generation,
                    format!(
                        "lean serve exited (code {:?}, signal {:?})",
                        payload.code, payload.signal
                    ),
                );
                return;
            }
            _ => {}
        }
    }
    on_exit(&app, generation, "lean serve closed its output".into());
}

fn on_exit(app: &AppHandle, generation: u64, reason: String) {
    let state = app.state::<Sidecar>();
    let restart = {
        let mut inner = state.inner.lock().unwrap();
        if inner.stopping || inner.generation != generation {
            return; // an older child; the current one is fine
        }
        inner.child = None;
        inner.stderr.push(reason.clone());
        let allowed = inner.budget.allow(Instant::now());
        if !allowed {
            inner.status = Status::Down {
                reason,
                stderr: inner.stderr.lines(),
            };
        }
        allowed
    };
    state.fail_pending();
    if restart {
        Sidecar::start(app);
        let _ = app.emit("serve_restarted", ());
    } else {
        let _ = app.emit("serve_status", state.status());
    }
}
