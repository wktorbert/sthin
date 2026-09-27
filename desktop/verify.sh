#!/usr/bin/env bash
# desktop/verify.sh — checks for the desktop app. Needs pnpm and a Rust
# toolchain; the root verify.sh stays Go-only and checks the Serve protocol the
# app depends on. CI runs both.
set -uo pipefail
cd "$(dirname "$0")"
export PATH="$HOME/.cargo/bin:$PATH"

PASSED=0
FAILED=0
check() {
  local name="$1"; shift
  if "$@" >/tmp/sthin-desktop-check.log 2>&1; then
    echo "  PASS  $name"; PASSED=$((PASSED + 1))
  else
    echo "  FAIL  $name"; sed 's/^/        /' /tmp/sthin-desktop-check.log | tail -n 25; FAILED=$((FAILED + 1))
  fi
}

# Tauri needs a sidecar for the host triple even to compile.
sidecar() { ../scripts/build-sidecar.sh; }
check "sidecar for this host"   sidecar
check "pnpm install"            pnpm install --frozen-lockfile
check "typescript typecheck"    pnpm -s typecheck
check "frontend unit tests"     pnpm -s test
check "frontend build"          pnpm -s vite build
check "rust fmt"                cargo fmt --manifest-path src-tauri/Cargo.toml --check
check "rust clippy"             cargo clippy --manifest-path src-tauri/Cargo.toml --all-targets -- -D warnings
check "rust unit tests"         cargo test --manifest-path src-tauri/Cargo.toml

echo
if [ "$FAILED" -eq 0 ]; then
  echo "DESKTOP-VERIFY: PASS ($PASSED/$((PASSED + FAILED)) checks)"
else
  echo "DESKTOP-VERIFY: FAIL ($PASSED/$((PASSED + FAILED)) checks)"; exit 1
fi
