#!/usr/bin/env bash
# live-check.sh — boot a real device slim, measure it, restore it, shut it down.
#
#   scripts/live-check.sh <ios|android> <id> [settle-seconds]
#
# Prints one final line:
#   LIVE-CHECK <platform> <id>: footprint=<MB>MB processes=<N> slim=<state> restored=<state>
# For Android, footprint is the dirty figure from `footprint -p` (host phys_footprint if absent).
# Exits 1 if any step fails. Whatever this script booted is shut down on exit, including on failure.
# Not part of verify.sh: it boots devices and takes minutes.

set -euo pipefail
cd "$(dirname "$0")/.."

PLATFORM="${1:?usage: live-check.sh <ios|android> <id> [settle-seconds]}"
ID="${2:?usage: live-check.sh <ios|android> <id> [settle-seconds]}"
SETTLE="${3:-60}"
LEAN="${LEAN_BIN:-bin/lean}"
TMP="$(mktemp -d)"

case "$PLATFORM" in ios|android) ;; *) echo "platform must be ios or android" >&2; exit 2 ;; esac
[ -x "$LEAN" ] || go build -o "$LEAN" ./cmd/lean

cleanup() {
  "$LEAN" shutdown "$ID" >/dev/null 2>&1 || true
  rm -rf "$TMP"
}
trap cleanup EXIT

# slim_of <list.json> prints the device's slim state.
slim_of() {
  python3 -c '
import json,sys
d=json.load(open(sys.argv[1]))
print(next(x["slim"] for x in d["devices"] if x["id"]==sys.argv[2]))' "$1" "$ID"
}

echo "== boot $PLATFORM $ID (slim)" >&2
"$LEAN" boot "$ID" >&2

echo "== settle ${SETTLE}s" >&2
sleep "$SETTLE"

"$LEAN" measure "$ID" --json > "$TMP/measure.json"
read -r FOOTPRINT PROCESSES < <(python3 -c '
import json,sys
m=json.load(open(sys.argv[1]))
fp=m.get("dirty_mb") if sys.argv[2]=="android" and m.get("dirty_mb") is not None else m["footprint_mb"]
print(fp, m["process_count"])' "$TMP/measure.json" "$PLATFORM")
cat "$TMP/measure.json" >&2

"$LEAN" list --json > "$TMP/list-slim.json" 2>/dev/null
SLIM="$(slim_of "$TMP/list-slim.json")"

# Targets from SPEC AC11; on a miss, show the top 10 processes so the miss is diagnosable.
MISS=0
if [ "$PLATFORM" = ios ]; then
  { [ "$FOOTPRINT" -le 1200 ] && [ "$PROCESSES" -lt 90 ]; } || MISS=1
else
  [ "$FOOTPRINT" -le 1600 ] || MISS=1
fi
if [ "$MISS" = 1 ]; then
  echo "== target missed; top 10 processes by footprint" >&2
  if [ "$PLATFORM" = ios ]; then
    ROOT="$(pgrep -f "$ID/data/var/run/launchd_bootstrap" | head -1)"
    ps -axo pid,ppid,comm > "$TMP/ps.txt"
    top -l 1 -stats pid,mem > "$TMP/top.txt"
    python3 - "$ROOT" "$TMP/ps.txt" "$TMP/top.txt" >&2 <<'PY'
import re,sys
root=int(sys.argv[1]); kids={}; names={}
for l in open(sys.argv[2]).read().splitlines()[1:]:
    p,pp,c=l.split(None,2); kids.setdefault(int(pp),[]).append(int(p)); names[int(p)]=c.rsplit('/',1)[-1]
mem={}; on=False
for l in open(sys.argv[3]):
    f=l.split()
    if f[:2]==['PID','MEM']: on=True; continue
    if on and len(f)>=2 and f[0].isdigit():
        m=re.match(r'([\d.]+)([BKMG]?)',f[1])
        if m: mem[int(f[0])]=float(m.group(1))*{'':1,'B':1,'K':1024,'M':1<<20,'G':1<<30}[m.group(2)]
tree=[root]; i=0
while i<len(tree): tree+=kids.get(tree[i],[]); i+=1
for p in sorted(tree,key=lambda p:-mem.get(p,0))[:10]:
    print(f"  {mem.get(p,0)/2**20:8.1f} MB  {p:>6}  {names.get(p,'?')}")
PY
  else
    footprint -p "$(pgrep -f "qemu-system.*-avd $ID" | head -1)" 2>/dev/null | sed -n '1,20p' >&2 || true
  fi
fi

echo "== restore" >&2
"$LEAN" restore "$ID" >&2
"$LEAN" list --json > "$TMP/list-restored.json" 2>/dev/null
RESTORED="$(slim_of "$TMP/list-restored.json")"

"$LEAN" shutdown "$ID" >&2

echo "LIVE-CHECK $PLATFORM $ID: footprint=${FOOTPRINT}MB processes=${PROCESSES} slim=${SLIM} restored=${RESTORED}"
[ "$SLIM" = slim ] && [ "$RESTORED" = stock ]
