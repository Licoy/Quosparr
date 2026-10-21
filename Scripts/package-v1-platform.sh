#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
export GOCACHE="${GOCACHE:-$ROOT/.build/v1-go-cache}"
TARGET="${1:?Usage: package-v1-platform.sh windows/amd64|windows/arm64|linux/amd64|linux/arm64}"
case "$TARGET" in windows/amd64|windows/arm64|linux/amd64|linux/arm64) ;; *) exit 2 ;; esac
WORK="${V1_BUILD_DIR:-$ROOT/build/v1-${TARGET//\//-}}"
DEVELOPMENT="${V1_DEVELOPMENT_PACKAGE:-0}"
case "$DEVELOPMENT" in 0|1) ;; *) printf 'V1_DEVELOPMENT_PACKAGE must be 0 or 1\n' >&2; exit 2 ;; esac
export MYGO_UPDATER_PRIVATE_KEY="${MYGO_UPDATE_PRIVATE_KEY:-${MYGO_UPDATER_PRIVATE_KEY:-}}"
if [[ "$DEVELOPMENT" == 0 ]]; then
  [[ -n "$MYGO_UPDATER_PRIVATE_KEY" ]] || { printf 'Missing MYGO_UPDATE_PRIVATE_KEY\n' >&2; exit 1; }
fi
cd "$ROOT"
mkdir -p "$ROOT/.build"
CONFIG_STAGE="$(mktemp -d "$ROOT/.build/v1-package-config.XXXXXX")"
trap 'rm -rf "$CONFIG_STAGE"' EXIT
[[ "$(go list -m -f '{{.Version}}' github.com/egoist/mygo)" == v0.6.2 ]]
V1_DEVELOPMENT_PACKAGE="$DEVELOPMENT" python3 "$ROOT/Scripts/stage-v1-config.py" "$ROOT" "$CONFIG_STAGE"
mkdir -p "$WORK/config"
cp "$CONFIG_STAGE/mygo.json" "$WORK/config/mygo.json"
if [[ "$DEVELOPMENT" == 1 ]]; then
  export GOFLAGS="${GOFLAGS:+$GOFLAGS }-tags=quosparr_dev_package"
fi
if [[ "$TARGET" == windows/* ]]; then
  MYGO_MODULE="$(go list -m -f '{{.Dir}}' github.com/egoist/mygo)"
  python3 "$ROOT/Scripts/prepare-windows-sqlite.py" "$MYGO_MODULE" "$CONFIG_STAGE"
  go -C "$CONFIG_STAGE/mygo-cli" run -mod=readonly ./cmd/mygo build -platform "$TARGET" -o "$WORK" "$CONFIG_STAGE"
else
  go run github.com/egoist/mygo/cmd/mygo build -platform "$TARGET" -o "$WORK" "$CONFIG_STAGE"
fi
STAGE="$WORK/${TARGET//\//-}"
EXECUTABLE="$STAGE/quosparr"
[[ "$TARGET" != windows/* ]] || EXECUTABLE="$STAGE/Quosparr.exe"
go version -m "$EXECUTABLE" | python3 -c 'import sys; tags=[line.strip().split("=",1)[1] for line in sys.stdin if line.strip().startswith("build\t-tags=")]; assert any("quosparr_dev_package" in value.split(",") for value in tags) == (sys.argv[1] == "1"), "packaged update mode mismatch"' "$DEVELOPMENT"
VERSION="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["version"])' "$WORK/config/mygo.json")"
if [[ "$TARGET" == windows/* ]]; then
  [[ ! -e "$STAGE/mygo-sqlite3.dll" ]] || { printf 'Windows SQLite must be embedded in the executable\n' >&2; exit 1; }
  [[ -f "$STAGE/Quosparr Setup $VERSION ${TARGET#*/}.exe" ]] || { printf 'NSIS installer missing; install makensis\n' >&2; exit 1; }
fi
if [[ "$DEVELOPMENT" == 1 ]]; then
  # Mygo writes unsigned Linux archives itself; Windows needs a review archive.
  if [[ "$TARGET" == windows/* ]]; then
    python3 "$ROOT/Scripts/verify-v1-platform.py" archive-windows "$WORK/config/mygo.json" "$STAGE" "$TARGET"
  fi
  go run ./Scripts/release-assets verify-payload -config "$WORK/config/mygo.json" -dir "$STAGE" -targets "${TARGET//\//-}"
  python3 "$ROOT/Scripts/verify-v1-platform.py" verify "$WORK/config/mygo.json" "$STAGE" "$TARGET"
  [[ ! -e "$STAGE/update-${TARGET//\//-}.json" ]] || { printf 'Development package must not contain an update manifest\n' >&2; exit 1; }
  printf 'Development package verified: %s (updater disabled)\n' "$STAGE"
  exit 0
fi
# Add the explicit SHA256 field and re-sign the unchanged CLI archive
# mygo v0.6.2 authenticates the SHA256 digest and ignores extra JSON fields
go run ./Scripts/release-assets sign -config "$WORK/config/mygo.json" -dir "$STAGE" -targets "${TARGET//\//-}"
go run ./Scripts/release-assets verify -config "$WORK/config/mygo.json" -dir "$STAGE" -targets "${TARGET//\//-}"
python3 "$ROOT/Scripts/verify-v1-platform.py" verify "$WORK/config/mygo.json" "$STAGE" "$TARGET"
printf 'Platform release assets: %s\n' "$STAGE"
