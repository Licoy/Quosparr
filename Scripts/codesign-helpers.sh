#!/usr/bin/env bash
# Sign with the Developer ID Application identity named by
# MACOS_DEV_IDENTITY (hardened runtime, secure timestamp, Apple's default
# designated requirement: identifier plus team). Without it, e.g. CI runs
# that have no signing secrets, sign ad hoc with a stable requirement.
#
# Default `codesign --sign -` records only a per-build cdhash. TCC keys
# kTCCServiceSystemPolicyAppData grants by that requirement, so "Allow"
# does not survive rebuilds (and often not even a relaunch of the same
# ad-hoc app). An identifier requirement stays stable across builds.

runway_designated_requirement() {
  local identifier="${1:?identifier required}"
  printf '=designated => identifier "%s"' "$identifier"
}

runway_codesign() {
  local target="${1:?target required}"
  local identifier="${2:?identifier required}"
  local entitlements="${3:-}"
  local args=(
    --force
    --options runtime
    --identifier "$identifier"
  )
  if [[ -n "${MACOS_DEV_IDENTITY:-}" ]]; then
    args+=(--timestamp --sign "$MACOS_DEV_IDENTITY")
  else
    args+=(--requirements "$(runway_designated_requirement "$identifier")" --sign -)
  fi
  if [[ -n "$entitlements" ]]; then
    args+=(--entitlements "$entitlements")
  fi
  codesign "${args[@]}" "$target"
}

# Notarize an app bundle or disk image with the App Store Connect API key in
# APPLE_API_KEY_PATH / APPLE_API_KEY_ID / APPLE_API_ISSUER_ID, then staple
# the ticket so Gatekeeper accepts it offline.
runway_notarize() {
  local target="${1:?target required}"
  local submission="$target" scratch= output status id
  : "${APPLE_API_KEY_PATH:?Missing APPLE_API_KEY_PATH}"
  : "${APPLE_API_KEY_ID:?Missing APPLE_API_KEY_ID}"
  : "${APPLE_API_ISSUER_ID:?Missing APPLE_API_ISSUER_ID}"
  if [[ -d "$target" ]]; then
    scratch="$(mktemp -d "${TMPDIR:-/tmp}/runway-notarize.XXXXXX")"
    submission="$scratch/$(basename "$target").zip"
    ditto -c -k --keepParent "$target" "$submission"
  fi
  local auth=(--key "$APPLE_API_KEY_PATH" --key-id "$APPLE_API_KEY_ID" --issuer "$APPLE_API_ISSUER_ID")
  output="$(xcrun notarytool submit "$submission" "${auth[@]}" --wait --output-format json)" || true
  [[ -z "$scratch" ]] || rm -rf "$scratch"
  # The verdict is the last (flat) JSON object; notarytool may print more first
  read -r status id < <(printf '%s' "$output" | python3 -c 'import json,sys; s=sys.stdin.read(); r=json.loads(s[s.rindex("{"):]); print(r.get("status") or "-", r.get("id") or "")' 2>/dev/null) || true
  if [[ "${status:-}" != Accepted ]]; then
    printf 'Notarization of %s failed (%s):\n%s\n' "$(basename "$target")" "${status:--}" "$output" >&2
    [[ -z "${id:-}" ]] || xcrun notarytool log "$id" "${auth[@]}" >&2 || true
    return 1
  fi
  xcrun stapler staple "$target"
  xcrun stapler validate "$target"
}

runway_codesign_self_test() {
  local scratch identifier app requirement observed
  scratch="$(mktemp -d "${TMPDIR:-/tmp}/runway-codesign.XXXXXX")"
  identifier="com.quosparr.app.codesign-self-test"
  app="$scratch/Dummy.app"
  mkdir -p "$app/Contents/MacOS"
  cp /bin/echo "$app/Contents/MacOS/Dummy"
  cat > "$app/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "https://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleExecutable</key>
  <string>Dummy</string>
  <key>CFBundleIdentifier</key>
  <string>com.quosparr.app.codesign-self-test</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
</dict>
</plist>
PLIST
  runway_codesign "$app" "$identifier"
  codesign --verify --deep --strict "$app"
  requirement="$(runway_designated_requirement "$identifier")"
  observed="$(codesign -d -r- "$app" 2>&1)"
  printf '%s\n' "$observed" | grep -Fq "designated => identifier \"$identifier\""
  if printf '%s\n' "$observed" | grep -Eq '^# designated => cdhash'; then
    printf 'codesign helper left an implicit cdhash requirement:\n%s\n' "$observed" >&2
    rm -rf "$scratch"
    return 1
  fi
  [[ "$requirement" == "=designated => identifier \"$identifier\"" ]]
  rm -rf "$scratch"
}

if [[ -n "${BASH_VERSION:-}" && "${BASH_SOURCE[0]:-}" == "${0:-}" ]]; then
  case "${1:-}" in
    --self-test)
      runway_codesign_self_test
      ;;
    *)
      printf 'Usage: %s --self-test\n' "$0" >&2
      exit 2
      ;;
  esac
fi
