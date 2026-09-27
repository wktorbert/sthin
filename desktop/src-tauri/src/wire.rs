//! Pure pieces of talking to `sthin serve`: classifying a stdout line and the
//! restart budget. Kept free of Tauri types so they are unit-testable.

use serde_json::{json, Value};
use std::collections::VecDeque;
use std::time::{Duration, Instant};

/// Error codes the Rust side adds to Serve's (see docs/serve-protocol.md and
/// src/serve/protocol.ts).
pub const CODE_SERVE_EXITED: i64 = -32001;
pub const CODE_SERVE_DOWN: i64 = -32002;
pub const CODE_DUPLICATE_ID: i64 = -32003;

/// One line from the sidecar's stdout.
#[derive(Debug, PartialEq)]
pub enum Incoming {
    /// A response to request `id`: Ok(result) or Err(error object).
    Response {
        id: u64,
        outcome: Result<Value, Value>,
    },
    /// A notification, forwarded to the frontend as-is.
    Notification { method: String, params: Value },
    /// Not JSON-RPC Sthin sends; kept for diagnostics.
    Garbage(String),
}

pub fn classify(line: &str) -> Incoming {
    let v: Value = match serde_json::from_str(line) {
        Ok(v) => v,
        Err(_) => return Incoming::Garbage(line.to_string()),
    };
    if let Some(method) = v.get("method").and_then(Value::as_str) {
        let params = v.get("params").cloned().unwrap_or(Value::Null);
        return Incoming::Notification {
            method: method.to_string(),
            params,
        };
    }
    let Some(id) = v.get("id").and_then(Value::as_u64) else {
        // A parse-error reply carries id null: nothing waits on it.
        return Incoming::Garbage(line.to_string());
    };
    if let Some(err) = v.get("error") {
        return Incoming::Response {
            id,
            outcome: Err(err.clone()),
        };
    }
    let result = v.get("result").cloned().unwrap_or(Value::Null);
    Incoming::Response {
        id,
        outcome: Ok(result),
    }
}

/// The request line written to the sidecar's stdin.
pub fn request_line(id: u64, method: &str, params: &Value) -> String {
    let mut line =
        json!({"jsonrpc": "2.0", "id": id, "method": method, "params": params}).to_string();
    line.push('\n');
    line
}

pub fn error(code: i64, message: &str) -> Value {
    json!({"code": code, "message": message})
}

/// Allows at most `max` restarts within `window`; after that the sidecar stays
/// down until the user retries, so a crash loop cannot eat a CPU.
pub struct RestartBudget {
    max: usize,
    window: Duration,
    recent: VecDeque<Instant>,
}

impl RestartBudget {
    pub fn new(max: usize, window: Duration) -> Self {
        Self {
            max,
            window,
            recent: VecDeque::new(),
        }
    }

    /// Records a restart at `now` and says whether it is allowed.
    pub fn allow(&mut self, now: Instant) -> bool {
        while let Some(&t) = self.recent.front() {
            if now.duration_since(t) >= self.window {
                self.recent.pop_front();
            } else {
                break;
            }
        }
        if self.recent.len() >= self.max {
            return false;
        }
        self.recent.push_back(now);
        true
    }

    pub fn reset(&mut self) {
        self.recent.clear();
    }
}

/// A bounded tail of the sidecar's stderr, shown when it will not stay up.
pub struct Tail {
    cap: usize,
    lines: VecDeque<String>,
}

impl Tail {
    pub fn new(cap: usize) -> Self {
        Self {
            cap,
            lines: VecDeque::new(),
        }
    }

    pub fn push(&mut self, line: String) {
        if self.lines.len() == self.cap {
            self.lines.pop_front();
        }
        self.lines.push_back(line);
    }

    pub fn lines(&self) -> Vec<String> {
        self.lines.iter().cloned().collect()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn classifies_responses_errors_and_notifications() {
        assert_eq!(
            classify(r#"{"jsonrpc":"2.0","id":7,"result":{"ok":true}}"#),
            Incoming::Response {
                id: 7,
                outcome: Ok(json!({"ok": true}))
            }
        );
        assert_eq!(
            classify(r#"{"jsonrpc":"2.0","id":8,"error":{"code":2,"message":"unknown device"}}"#),
            Incoming::Response {
                id: 8,
                outcome: Err(json!({"code": 2, "message": "unknown device"}))
            }
        );
        assert_eq!(
            classify(
                r#"{"jsonrpc":"2.0","method":"progress","params":{"id":7,"stage":{"name":"boot","status":"ok"}}}"#
            ),
            Incoming::Notification {
                method: "progress".into(),
                params: json!({"id": 7, "stage": {"name": "boot", "status": "ok"}})
            }
        );
    }

    #[test]
    fn treats_non_protocol_lines_as_garbage() {
        assert!(matches!(classify("panic: oh no"), Incoming::Garbage(_)));
        assert!(matches!(
            classify(
                r#"{"jsonrpc":"2.0","id":null,"error":{"code":-32700,"message":"parse error"}}"#
            ),
            Incoming::Garbage(_)
        ));
    }

    #[test]
    fn request_line_is_one_json_line() {
        let l = request_line(3, "boot", &json!({"id": "U1"}));
        assert!(l.ends_with('\n') && !l.trim_end().contains('\n'));
        let v: Value = serde_json::from_str(l.trim_end()).unwrap();
        assert_eq!(v["id"], 3);
        assert_eq!(v["method"], "boot");
        assert_eq!(v["params"]["id"], "U1");
    }

    #[test]
    fn restart_budget_allows_three_per_minute_then_recovers() {
        let mut b = RestartBudget::new(3, Duration::from_secs(60));
        let t0 = Instant::now();
        assert!(b.allow(t0));
        assert!(b.allow(t0 + Duration::from_secs(1)));
        assert!(b.allow(t0 + Duration::from_secs(2)));
        assert!(!b.allow(t0 + Duration::from_secs(3)));
        // Once the first restart is a minute old, one more is allowed.
        assert!(b.allow(t0 + Duration::from_secs(60)));
        assert!(!b.allow(t0 + Duration::from_millis(60_500)));
        b.reset();
        assert!(b.allow(t0 + Duration::from_secs(62)));
    }

    #[test]
    fn tail_keeps_the_last_lines() {
        let mut t = Tail::new(2);
        for s in ["a", "b", "c"] {
            t.push(s.into());
        }
        assert_eq!(t.lines(), vec!["b", "c"]);
    }
}
