#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
export GOCACHE="${GOCACHE:-$ROOT/.build/v1-go-cache}"
DIST="${1:?asset directory required}"
CONFIG="${2:-$ROOT/mygo.json}"
TARGETS="${3:-darwin-arm64,darwin-amd64,windows-amd64,windows-arm64,linux-amd64,linux-arm64}"
cd "$ROOT"
python3 "$ROOT/Scripts/release-checksums.py" verify "$DIST"
go run ./Scripts/release-assets verify -config "$CONFIG" -dir "$DIST" -targets "$TARGETS"
if [[ "$TARGETS" == *darwin-* ]]; then
  VERSION="$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["version"])' "$CONFIG")"
  MAC_ARCHES=
  [[ "$TARGETS" != *darwin-arm64* ]] || MAC_ARCHES=arm64
  if [[ "$TARGETS" == *darwin-amd64* ]]; then MAC_ARCHES="${MAC_ARCHES:+$MAC_ARCHES,}x86_64"; fi
  python3 "$ROOT/Scripts/verify-legacy-appcast.py" "$DIST" "$CONFIG" "${V1_RELEASE_TAG:-v$VERSION}" "$MAC_ARCHES"
fi
python3 "$ROOT/Scripts/verify-v1-installers.py" "$DIST" "$CONFIG" "$TARGETS"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/runway-release-verify.XXXXXX")"
MOUNT=
cleanup() {
  if [[ -n "$MOUNT" ]]; then hdiutil detach "$MOUNT" >/dev/null; fi
  rm -rf "$WORK"
}
trap cleanup EXIT
# Release apps and disk images are Developer ID signed with a stapled ticket
verify_notarized() {
  [[ "$(codesign -dvv "$1" 2>&1)" == *$'\nAuthority=Developer ID Application: '* ]] || {
    printf 'Not signed with Developer ID: %s\n' "$1" >&2
    return 1
  }
  xcrun stapler validate "$1" >/dev/null
}
VERSION="$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["version"])' "$CONFIG")"
IFS=',' read -r -a SELECTED <<< "$TARGETS"
for target in "${SELECTED[@]}"; do
  [[ "$target" == darwin-* ]] || continue
  [[ "$(uname -s)" == Darwin ]] || { printf 'macOS signature verification requires macOS\n' >&2; exit 1; }
  ARCH="${target#darwin-}"
  [[ "$ARCH" != amd64 ]] || ARCH=x86_64
  mkdir -p "$WORK/$target/tar" "$WORK/$target/zip"
  tar -xzf "$DIST/quosparr-$VERSION-$target.tar.gz" -C "$WORK/$target/tar"
  ditto -x -k "$DIST/Quosparr-macos-$ARCH.zip" "$WORK/$target/zip"
  bash "$ROOT/Scripts/verify-v1-app.sh" "$WORK/$target/tar/Quosparr.app" "$ARCH" "$CONFIG"
  codesign --verify --deep --strict "$WORK/$target/zip/Quosparr.app"
  verify_notarized "$WORK/$target/zip/Quosparr.app"
  diff -qr "$WORK/$target/tar/Quosparr.app" "$WORK/$target/zip/Quosparr.app"
  hdiutil verify "$DIST/Quosparr-macos-$ARCH.dmg" >/dev/null
  verify_notarized "$DIST/Quosparr-macos-$ARCH.dmg"
  MOUNT="$WORK/$target/mount"
  mkdir -p "$MOUNT"
  hdiutil attach "$DIST/Quosparr-macos-$ARCH.dmg" -mountpoint "$MOUNT" -readonly -nobrowse >/dev/null
  codesign --verify --deep --strict "$MOUNT/Quosparr.app"
  diff -qr "$WORK/$target/tar/Quosparr.app" "$MOUNT/Quosparr.app"
  hdiutil detach "$MOUNT" >/dev/null
  MOUNT=
done
printf 'Selected release assets fully verified\n'
