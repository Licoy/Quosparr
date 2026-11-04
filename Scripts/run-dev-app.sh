#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

usage() {
  cat <<'EOF'
Usage: Scripts/run-dev-app.sh [--widget] [--tier-badges] [app options]

Build the Go (mygo) app with the release identity (Quosparr,
com.quosparr.app) and run it in the foreground with the real HOME, data and
preferences. The updater is disabled (development package). Ctrl+C quits.
Quit the installed Quosparr first; a previous run of this script is replaced.

Options:
  -h, --help
      Show this help and exit.

  --widget
      Build the full development package with the WidgetKit extension
      (V1_DEVELOPMENT_PACKAGE=1 Scripts/package-v1-app.sh) and run that.

  --tier-badges
      Also open a window with every Codex, Grok and Claude subscription
      badge in the light and the dark look (QUOSPARR_DEV_TIER_BADGES=1).

Other options go to the app, e.g. --settings or --self-check.
Env: ARCH=arm64|x86_64 (default: this Mac).
     MACOS_DEV_IDENTITY=<Developer ID Application identity> signs with that
     certificate from the keychain (default: ad hoc).

Examples:
  Scripts/run-dev-app.sh
  Scripts/run-dev-app.sh --settings
  Scripts/run-dev-app.sh --widget
  Scripts/run-dev-app.sh --tier-badges
EOF
}

WIDGET=0
APP_ARGS=()
for arg in "$@"; do
  case "$arg" in
    -h|--help) usage; exit 0 ;;
    --widget) WIDGET=1 ;;
    --tier-badges) export QUOSPARR_DEV_TIER_BADGES=1 ;;
    *) APP_ARGS+=("$arg") ;;
  esac
done

ARCH="${ARCH:-$(uname -m)}"
case "$ARCH" in
  arm64) GOARCH=arm64 ;;
  x86_64) GOARCH=amd64 ;;
  *) printf 'Unsupported architecture: %s\n' "$ARCH" >&2; exit 2 ;;
esac
if [[ "$WIDGET" == 1 ]]; then
  WORK="$ROOT/build/v1-$ARCH"
else
  WORK="$ROOT/.build/v1-dev-run-$ARCH"
fi
APP="$WORK/darwin-$GOARCH/Quosparr.app"
EXE="$APP/Contents/MacOS/Quosparr"

# Replace a previous run of this script; refuse to start next to any other
# Quosparr, which holds the single-instance lock and would take over.
LOCKS=("$HOME/.quosparr/quosparr.lock" "$HOME/Library/Application Support/Quosparr/quosparr.lock")
for lock in "${LOCKS[@]}"; do
  [[ -e "$lock" ]] || continue
  while IFS= read -r pid; do
    [[ -n "$pid" ]] || continue
    executable="$(ps -p "$pid" -o comm= | sed 's/^[[:space:]]*//')"
    case "$executable" in
      "$ROOT"/.build/v1-dev-run-*/Quosparr.app/Contents/MacOS/Quosparr|\
      "$ROOT"/build/v1-*/Quosparr.app/Contents/MacOS/Quosparr)
        kill "$pid" 2>/dev/null || true ;;
      *)
        printf 'Quosparr is already running (%s). Quit it first.\n' "${executable:-pid $pid}" >&2
        exit 1 ;;
    esac
  done < <(lsof -t "$lock" 2>/dev/null || true)
done

cd "$ROOT"
if [[ "$WIDGET" == 1 ]]; then
  ARCH="$ARCH" V1_DEVELOPMENT_PACKAGE=1 bash "$ROOT/Scripts/package-v1-app.sh"
else
  # The same config staging and update mode as the development package,
  # without the Widget extension, so a rebuild stays quick.
  source "$ROOT/Scripts/codesign-helpers.sh"
  STAGE="$WORK/config"
  rm -rf "$STAGE"
  V1_DEVELOPMENT_PACKAGE=1 python3 "$ROOT/Scripts/stage-v1-config.py" "$ROOT" "$STAGE"
  GOFLAGS="${GOFLAGS:+$GOFLAGS }-tags=quosparr_dev_package" \
    go run github.com/egoist/mygo/cmd/mygo build \
    -platform "darwin/$GOARCH" -o "$WORK" -skip-dmg -skip-notarize -sign - "$STAGE"
  rm -f "$WORK/darwin-$GOARCH"/update-*.json "$WORK/darwin-$GOARCH"/*.tar.gz
  [[ -x "$EXE" ]] || { printf 'Missing MyGo bundle: %s\n' "$APP" >&2; exit 1; }

  # The app always loads the WidgetKit bridge; without the extension it only
  # has no widgets to reload.
  BRIDGE="$APP/Contents/Frameworks/libQuosparrWidgetBridge.dylib"
  mkdir -p "$APP/Contents/Frameworks"
  xcrun swiftc -swift-version 5 -O -emit-library \
    -module-cache-path "$WORK/bridge-module-cache" \
    -target "$ARCH-apple-macosx12.0" -module-name QuosparrWidgetBridge \
    -Xlinker -install_name -Xlinker @rpath/libQuosparrWidgetBridge.dylib \
    "$ROOT/Sources/QuosparrWidgetBridge/WidgetBridge.swift" -o "$BRIDGE"

  # Same signing as package-v1-app.sh (Developer ID with MACOS_DEV_IDENTITY,
  # else a stable ad-hoc requirement), so Keychain and privacy grants survive
  # rebuilds.
  ENTITLEMENTS="$WORK/app-entitlements.plist"
  rm -f "$ENTITLEMENTS"
  plutil -create xml1 "$ENTITLEMENTS"
  /usr/libexec/PlistBuddy -c 'Add :com.apple.security.cs.disable-library-validation bool true' "$ENTITLEMENTS"
  HOST_ID="$(plutil -extract CFBundleIdentifier raw "$APP/Contents/Info.plist")"
  runway_codesign "$BRIDGE" "$HOST_ID.widget-bridge"
  runway_codesign "$APP/Contents/Resources/libmygo-sqlite3.dylib" "$HOST_ID.sqlite"
  runway_codesign "$APP" "$HOST_ID" "$ENTITLEMENTS"
  codesign --verify --deep --strict "$APP"
fi

printf 'Quosparr Dev: %s\n' "$APP"
exec "$EXE" ${APP_ARGS[@]+"${APP_ARGS[@]}"}
