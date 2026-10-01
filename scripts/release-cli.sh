#!/usr/bin/env bash
# release-cli.sh — build the sthin CLI for every supported platform and write
# release archives plus a sha256 checksum file, the way a GitHub Release ships them.
#
# Usage: scripts/release-cli.sh <version> [outdir]      (default outdir: dist)
# Produces:
#   dist/sthin_<version>_<os>_<arch>.tar.gz   (zip on windows) containing
#     sthin[.exe], README.md, LICENSE, NOTICE, completions/ (bash, zsh, fish, powershell)
#   dist/sthin-<version>.<tag>.bottle.tar.gz   Homebrew bottle per macOS/Linux target
#     (sthin/<version>/{bin,etc,share} laid out the way `brew install` pours it)
#   dist/checksums.txt                         sha256 of every archive and bottle
#
# Why bottles: a formula without one counts as a source build, and Homebrew then
# insists on a compiler toolchain (Command Line Tools on macOS, gcc on Linux)
# even though the formula only copies a prebuilt binary. A poured bottle skips
# those checks and never runs the formula's install block, so completions are
# placed in the bottle at Homebrew's own paths instead.
set -euo pipefail
cd "$(dirname "$0")/.."
version="${1:?version, e.g. 0.1.0}"
out="${2:-dist}"
rm -rf "$out"; mkdir -p "$out"

# Completion scripts come from a host build, since the cross-built binaries
# cannot run here. They are the same for every platform.
comp="$out/completions"; mkdir -p "$comp"
go build -trimpath -ldflags "-X main.version=$version" -o "$out/sthin-host" ./cmd/sthin
"$out/sthin-host" completion bash > "$comp/sthin.bash"
"$out/sthin-host" completion zsh > "$comp/_sthin"
"$out/sthin-host" completion fish > "$comp/sthin.fish"
"$out/sthin-host" completion powershell > "$comp/sthin.ps1"
rm -f "$out/sthin-host"

# Homebrew bottle tag per target. macOS bottles are tagged with the oldest
# macOS they are meant for: Homebrew pours an older tag on any newer macOS, so
# one bottle per architecture covers Sonoma, Sequoia, Tahoe and later.
bottle_tag() {
  case "$1" in
    darwin/arm64) echo arm64_sonoma ;;
    darwin/amd64) echo sonoma ;;
    linux/amd64)  echo x86_64_linux ;;
    linux/arm64)  echo arm64_linux ;;
    *) echo "" ;;
  esac
}

# Write a bottle for a built binary: sthin/<version>/bin/sthin plus the
# completion scripts where Homebrew looks for them. The file name is what
# Homebrew requests for a formula's `root_url`: sthin-<version>.<tag>.bottle.tar.gz
# (one dash; the double-dash form is only for GitHub Packages).
bottle() {
  local bin="$1" tag="$2" keg="$out/bottle_$tag/sthin/$version"
  mkdir -p "$keg/bin" "$keg/etc/bash_completion.d" \
    "$keg/share/zsh/site-functions" "$keg/share/fish/vendor_completions.d"
  cp "$bin" "$keg/bin/sthin"
  cp "$comp/sthin.bash" "$keg/etc/bash_completion.d/sthin"
  cp "$comp/_sthin" "$keg/share/zsh/site-functions/_sthin"
  cp "$comp/sthin.fish" "$keg/share/fish/vendor_completions.d/sthin.fish"
  tar -C "$out/bottle_$tag" -czf "$out/sthin-${version}.${tag}.bottle.tar.gz" sthin
  rm -rf "$out/bottle_$tag"
  echo "built bottle $tag"
}

targets="darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64"
for p in $targets; do
  os="${p%/*}"; arch="${p#*/}"; ext=""; [ "$os" = windows ] && ext=".exe"
  stage="$out/stage_${os}_${arch}"; mkdir -p "$stage"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath \
    -ldflags "-s -w -X main.version=$version" -o "$stage/sthin$ext" ./cmd/sthin
  tag="$(bottle_tag "$p")"
  [ -n "$tag" ] && bottle "$stage/sthin" "$tag"
  cp README.md LICENSE NOTICE "$stage/"
  cp -R "$comp" "$stage/completions"
  name="sthin_${version}_${os}_${arch}"
  if [ "$os" = windows ]; then
    (cd "$stage" && zip -q -r "../$name.zip" .)
  else
    tar -C "$stage" -czf "$out/$name.tar.gz" .
  fi
  rm -rf "$stage"
  echo "built $name"
done

(cd "$out" && shasum -a 256 sthin_* sthin-*.bottle.tar.gz > checksums.txt)
echo "checksums:"; cat "$out/checksums.txt"
