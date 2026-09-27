#!/usr/bin/env bash
# release-cli.sh — build the sthin CLI for every supported platform and write
# release archives plus a sha256 checksum file, the way a GitHub Release ships them.
#
# Usage: scripts/release-cli.sh <version> [outdir]      (default outdir: dist)
# Produces:
#   dist/sthin_<version>_<os>_<arch>.tar.gz   (zip on windows) containing
#     sthin[.exe], README.md, LICENSE, NOTICE
#   dist/checksums.txt                         sha256 of every archive
set -euo pipefail
cd "$(dirname "$0")/.."
version="${1:?version, e.g. 0.1.0}"
out="${2:-dist}"
rm -rf "$out"; mkdir -p "$out"

targets="darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64"
for p in $targets; do
  os="${p%/*}"; arch="${p#*/}"; ext=""; [ "$os" = windows ] && ext=".exe"
  stage="$out/stage_${os}_${arch}"; mkdir -p "$stage"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath \
    -ldflags "-s -w -X main.version=$version" -o "$stage/sthin$ext" ./cmd/sthin
  cp README.md LICENSE NOTICE "$stage/"
  name="sthin_${version}_${os}_${arch}"
  if [ "$os" = windows ]; then
    (cd "$stage" && zip -q -r "../$name.zip" .)
  else
    tar -C "$stage" -czf "$out/$name.tar.gz" .
  fi
  rm -rf "$stage"
  echo "built $name"
done

(cd "$out" && shasum -a 256 sthin_* > checksums.txt)
echo "checksums:"; cat "$out/checksums.txt"
