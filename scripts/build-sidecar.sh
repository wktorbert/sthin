#!/usr/bin/env bash
# build-sidecar.sh — build the `lean` binary the desktop app bundles as its
# sidecar, into desktop/src-tauri/binaries/lean-<target-triple>[.exe] as Tauri's
# externalBin requires (https://v2.tauri.app/develop/sidecar/).
#
# The version comes from desktop/src-tauri/tauri.conf.json and is stamped into
# the binary, so the app's version and the sidecar's always match (the app
# refuses a mismatched sidecar).
#
# Usage:
#   scripts/build-sidecar.sh            # the host's own triple (rustc -vV)
#   scripts/build-sidecar.sh all        # every triple the app ships for
#   scripts/build-sidecar.sh aarch64-apple-darwin x86_64-pc-windows-msvc
set -euo pipefail
cd "$(dirname "$0")/.."

out=desktop/src-tauri/binaries
conf=desktop/src-tauri/tauri.conf.json
version="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["version"])' "$conf")"
mkdir -p "$out"

goos_goarch() {
  case "$1" in
    aarch64-apple-darwin)     echo "darwin arm64" ;;
    x86_64-apple-darwin)      echo "darwin amd64" ;;
    x86_64-pc-windows-msvc)   echo "windows amd64" ;;
    aarch64-pc-windows-msvc)  echo "windows arm64" ;;
    x86_64-unknown-linux-gnu) echo "linux amd64" ;;
    *) echo "unsupported triple: $1" >&2; return 1 ;;
  esac
}

triples=("$@")
if [ ${#triples[@]} -eq 0 ]; then
  triples=("$(rustc -vV | sed -n 's/^host: //p')")
elif [ "${triples[0]}" = "all" ]; then
  triples=(aarch64-apple-darwin x86_64-apple-darwin x86_64-pc-windows-msvc)
fi

for t in "${triples[@]}"; do
  read -r goos goarch < <(goos_goarch "$t")
  ext=""
  [ "$goos" = "windows" ] && ext=".exe"
  dest="$out/lean-$t$ext"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -trimpath -ldflags "-s -w -X main.version=$version" -o "$dest" ./cmd/lean
  echo "built $dest ($version)"
done
