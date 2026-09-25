#!/usr/bin/env bash
# verify.sh — the single source of truth for "is it done?"
#
# Final line is read by the /goal evaluator. Do not change the marker format.
# Do not weaken checks to make this pass. Checks map to AC1–AC10 in
# .oneshot/SPEC.md. AC11 (live boot numbers) is deliberately not here.
#
# Only read-only host commands run here: simctl list, avd file reads, adb devices.
# Nothing in this script boots a device.

set -uo pipefail   # deliberately NOT -e: we want every check to run
cd "$(dirname "$0")"

PASSED=0
FAILED=0
FIRST_FAILURE=""
LOG=/tmp/oneshot-check.log

check() {
  local name="$1"; shift
  if "$@" >"$LOG" 2>&1; then
    echo "  PASS  $name"
    PASSED=$((PASSED + 1))
  else
    echo "  FAIL  $name"
    sed 's/^/        /' "$LOG" | tail -n 25
    FAILED=$((FAILED + 1))
    [ -z "$FIRST_FAILURE" ] && FIRST_FAILURE="$name"
  fi
}

# Put the Android SDK's adb on PATH the same way a user's shell would, so the
# smoke checks see the toolchain doctor is expected to find.
export PATH="$PATH:$HOME/Library/Android/sdk/platform-tools:$HOME/Library/Android/sdk/emulator"

echo "── static ─────────────────────────────────"
gofmt_clean() { [ -f go.mod ] && [ -z "$(gofmt -l . 2>&1)" ]; }
check "AC1 gofmt clean"          gofmt_clean
check "AC1 go vet"               go vet ./...

echo "── unit ───────────────────────────────────"
check "AC2 go test"              go test ./... -count=1

echo "── build ──────────────────────────────────"
check "AC3 build darwin"         go build -o bin/lean ./cmd/lean
linux_build() { GOOS=linux GOARCH=amd64 go build ./...; }
check "AC3 build linux"          linux_build
windows_build() { GOOS=windows GOARCH=amd64 go build ./...; }
check "AC3 build windows"        windows_build

echo "── smoke ──────────────────────────────────"
help_lists_commands() {
  local out; out="$(bin/lean --help)" || return 1
  for c in list boot restore shutdown measure doctor profile version run logs mcp serve lease release leases adb; do
    grep -qE "^\s+$c\b" <<<"$out" || { echo "missing subcommand: $c"; return 1; }
  done
  out="$(bin/lean boot --help)" || return 1
  for f in --stock --except --ram; do
    grep -q -- "$f" <<<"$out" || { echo "boot --help missing $f"; return 1; }
  done
}
check "AC4 help lists commands"  help_lists_commands

list_json() {
  bin/lean list --json > /tmp/oneshot-list.json || return 1
  python3 - <<'PY'
import json,sys
d=json.load(open("/tmp/oneshot-list.json"))
assert d.get("schema_version")==1, "schema_version != 1"
devs=d.get("devices") or []
plats={x.get("platform") for x in devs}
assert "ios" in plats, "no ios device listed"
assert "android" in plats, "no android device listed"
for x in devs:
    for k in ("id","name","platform","state","slim"):
        assert k in x, f"device missing {k}: {x}"
    assert x["slim"] in ("stock","slim","partial","unknown","n/a"), x["slim"]
    assert x.get("kind") in ("simulator","emulator","physical"), x
print(f"ok: {len(devs)} devices")
PY
}
check "AC5 list --json"          list_json

profile_json() {
  for p in ios android; do
    bin/lean profile "$p" --json > "/tmp/oneshot-profile-$p.json" || return 1
  done
  python3 - <<'PY'
import json
must={"ios":"com.apple.sharingd","android":"com.google.android.bluetooth"}
for p,label in must.items():
    d=json.load(open(f"/tmp/oneshot-profile-{p}.json"))
    assert "validated_against" in d, f"{p}: no validated_against"
    cats=d.get("categories") or []
    assert len(cats)>=8, f"{p}: only {len(cats)} categories"
    nd=set(d.get("never_disable") or [])
    assert label in nd, f"{p}: {label} not in never_disable"
    for c in cats:
        assert c.get("items"), f"{p}: empty category {c.get('id')}"
        assert not (set(c["items"]) & nd), f"{p}: category {c['id']} contains a never-disable item"
    print(f"ok: {p} {len(cats)} categories")
PY
}
check "AC6 profile --json"       profile_json

doctor_json() {
  bin/lean doctor --json > /tmp/oneshot-doctor.json
  local rc=$?
  python3 - <<'PY'
import json
d=json.load(open("/tmp/oneshot-doctor.json"))
assert d.get("schema_version")==1
checks=d.get("checks") or []
assert len(checks)>=5, "too few checks"
for c in checks:
    assert c.get("status") in ("ok","warn","fail","n/a"), c
print("ok:", ", ".join(f"{c['name']}={c['status']}" for c in checks))
PY
  [ $rc -eq 0 ] || { echo "doctor exited $rc (expected 0 on this Mac)"; return 1; }
}
check "AC7 doctor --json"        doctor_json

measure_shutdown_exit2() {
  local udid
  bin/lean list --json > /tmp/oneshot-list-ac8.json || { echo "list --json failed"; return 1; }
  udid="$(python3 -c '
import json
d=json.load(open("/tmp/oneshot-list-ac8.json"))
x=[v for v in d["devices"] if v["platform"]=="ios" and v["state"]=="shutdown"]
print(x[0]["id"] if x else "")')"
  [ -n "$udid" ] || { echo "no shutdown ios device to test against"; return 1; }
  bin/lean measure "$udid" >/tmp/oneshot-measure.out 2>&1
  local rc=$?
  cat /tmp/oneshot-measure.out
  [ $rc -eq 2 ] || { echo "expected exit 2, got $rc"; return 1; }
  grep -qi "not booted" /tmp/oneshot-measure.out
}
check "AC8 measure on shutdown device exits 2" measure_shutdown_exit2

tui_tests_exist() {
  [ -d internal/tui ] || { echo "internal/tui missing"; return 1; }
  ls internal/tui/*_test.go >/dev/null 2>&1 || { echo "no TUI tests"; return 1; }
  # Capture to a file before grepping: `go test -v | grep -q` under pipefail
  # fails with SIGPIPE (141) whenever grep matches before go test finishes.
  go test ./internal/tui/ -count=1 -v >/tmp/oneshot-tui.out 2>&1 || { tail -n 20 /tmp/oneshot-tui.out; return 1; }
  # SPEC AC9: both platforms render, `?` help overlay, `r` confirm, Enter boots.
  for t in TestRendersBothPlatforms TestHelpOverlay TestRestoreConfirm TestEnterBootsAndShowsEachStage; do
    grep -qE "^--- PASS: $t " /tmp/oneshot-tui.out || { echo "AC9 test not passing: $t"; return 1; }
  done
  # non-TTY fallback: `lean` with piped stdout behaves like `lean list`
  bin/lean </dev/null >/tmp/oneshot-lean-notty.out 2>&1 || { echo "bare lean exited non-zero without a TTY"; return 1; }
  grep -qiE 'ios|android' /tmp/oneshot-lean-notty.out
}
check "AC9 tui model tests + non-tty fallback" tui_tests_exist

marker_unique() {
  local hits
  hits="$(grep -rl 'ONESHOT-VERIFY' . --exclude-dir=.git --exclude-dir=.oneshot --exclude-dir=bin --exclude=verify.sh 2>/dev/null || true)"
  [ -z "$hits" ] || { echo "marker found outside verify.sh:"; echo "$hits"; return 1; }
}
check "AC10 marker unique"       marker_unique

mcp_smoke() {
  # Speak MCP over stdio to the real binary: handshake, tool list, one read-only call.
  python3 scripts/mcp-client.py bin/lean devices_list > /tmp/oneshot-mcp.out || { cat /tmp/oneshot-mcp.out; return 1; }
  python3 - <<'PYCHECK'
import json
lines=[json.loads(l) for l in open("/tmp/oneshot-mcp.out") if l.strip()]
tools=set(lines[0]["tools"])
for want in ("devices_list","boot","shutdown","restore","measure","screenshot","tap","install","launch","logs","lease","release","leases"):
    assert want in tools, "missing tool "+want
call=lines[1]
assert not call["isError"], call
d=json.loads(call["text"])
assert d["schema_version"]==1 and any(x["platform"]=="ios" for x in d["devices"]), d
print("ok:", len(tools), "tools;", len(d["devices"]), "devices via MCP")
PYCHECK
}
check "AC12 mcp stdio smoke"     mcp_smoke

serve_smoke() {
  # Speak the desktop app's protocol to the real binary: initialize, then a
  # read-only listing, doctor, and one usage error that must map to code 2.
  python3 scripts/serve-client.py bin/lean devices_list doctor > /tmp/oneshot-serve.out || { cat /tmp/oneshot-serve.out; return 1; }
  python3 scripts/serve-client.py bin/lean boot '{"id":"no-such-device-lean-verify"}' > /tmp/oneshot-serve-err.out
  python3 - <<'PYCHECK'
import json
calls={}
for l in open("/tmp/oneshot-serve.out"):
    m=json.loads(l)
    if "method" in m: calls[m["method"]]=m
init=calls["initialize"]["result"]
assert init["schema_version"]==1 and init["lean_version"], init
assert {p["platform"] for p in init["platforms"]}=={"ios","android"}, init
d=calls["devices_list"]["result"]
assert d["schema_version"]==1 and any(x["platform"]=="ios" for x in d["devices"]), d
doc=calls["doctor"]["result"]
assert doc["blocking"] is False and len(doc["checks"])>=5, doc
err=[json.loads(l) for l in open("/tmp/oneshot-serve-err.out") if '"boot"' in l][0]["error"]
assert err["code"]==2 and "unknown device" in err["message"], err
print("ok:", len(d["devices"]), "devices via serve; unknown device -> code 2")
PYCHECK
}
check "AC13 serve stdio smoke"   serve_smoke

TOTAL=$((PASSED + FAILED))
echo
if [ "$FAILED" -eq 0 ] && [ "$TOTAL" -gt 0 ]; then
  echo "ONESHOT-VERIFY: PASS ($PASSED/$TOTAL checks)"
  exit 0
elif [ "$TOTAL" -eq 0 ]; then
  echo "ONESHOT-VERIFY: FAIL (0/0 checks) — harness has no checks defined"
  exit 1
else
  echo "ONESHOT-VERIFY: FAIL ($PASSED/$TOTAL checks) — $FIRST_FAILURE"
  exit 1
fi
