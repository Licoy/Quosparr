#!/usr/bin/env bash
# CI only: import the Developer ID Application certificate into a throwaway
# keychain and write the App Store Connect API key for notarization.
#
# MACOS_CERTIFICATE_P12 is the base64 .p12 exported from Keychain Access.
# Its password is read by openssl from the environment and never reaches a
# command line. Only the env lines for $GITHUB_ENV go to stdout: the API key
# path and MACOS_DEV_IDENTITY, read from the certificate itself.
set -euo pipefail
exec 3>&1 1>&2

for key in MACOS_CERTIFICATE_P12 MACOS_CERTIFICATE_PASSWORD APPLE_API_KEY_P8; do
  [[ -n "${!key:-}" ]] || { printf 'Missing %s\n' "$key" >&2; exit 1; }
done
umask 077
WORK="$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/quosparr-signing.XXXXXX")"
trap 'rm -f "$WORK"/*.p12 "$WORK"/*.pem "$WORK"/*.cer' EXIT
KEYCHAIN="$WORK/signing.keychain-db"
# Random, and it only guards this keychain on a disposable runner
KEYCHAIN_PASSWORD="$(/usr/bin/openssl rand -hex 24)"

printf '%s' "$MACOS_CERTIFICATE_P12" | /usr/bin/base64 -D > "$WORK/certificate.p12"
/usr/bin/openssl pkcs12 -in "$WORK/certificate.p12" -passin env:MACOS_CERTIFICATE_PASSWORD -nokeys -clcerts -out "$WORK/certificate.pem"
/usr/bin/openssl pkcs12 -in "$WORK/certificate.p12" -passin env:MACOS_CERTIFICATE_PASSWORD -nocerts -nodes -out "$WORK/key.pem"
# The exported .p12 usually holds only the leaf; codesign needs the chain
/usr/bin/curl -fsSL https://www.apple.com/certificateauthority/DeveloperIDG2CA.cer -o "$WORK/DeveloperIDG2CA.cer"

/usr/bin/security create-keychain -p "$KEYCHAIN_PASSWORD" "$KEYCHAIN"
/usr/bin/security set-keychain-settings -lut 21600 "$KEYCHAIN"
/usr/bin/security unlock-keychain -p "$KEYCHAIN_PASSWORD" "$KEYCHAIN"
/usr/bin/security import "$WORK/DeveloperIDG2CA.cer" -k "$KEYCHAIN"
/usr/bin/security import "$WORK/certificate.pem" -k "$KEYCHAIN"
/usr/bin/security import "$WORK/key.pem" -k "$KEYCHAIN" -T /usr/bin/codesign
/usr/bin/security set-key-partition-list -S apple-tool:,apple: -s -k "$KEYCHAIN_PASSWORD" "$KEYCHAIN" >/dev/null
SEARCH=()
while IFS= read -r line; do
  line="${line#"${line%%[![:space:]]*}"}"
  SEARCH+=("${line//\"/}")
done < <(/usr/bin/security list-keychains -d user)
/usr/bin/security list-keychains -d user -s "$KEYCHAIN" ${SEARCH[@]+"${SEARCH[@]}"}
IDENTITY="$(/usr/bin/security find-identity -v -p codesigning "$KEYCHAIN" | sed -n 's/.*"\(Developer ID Application: [^"]*\)"$/\1/p' | head -n 1)"
[[ -n "$IDENTITY" ]] || {
  printf 'The certificate holds no valid Developer ID Application identity\n' >&2
  exit 1
}

# APPLE_API_KEY_P8 is the base64 .p8, like the .p12
printf '%s' "$APPLE_API_KEY_P8" | /usr/bin/base64 -D > "$WORK/AuthKey.p8"
/usr/bin/openssl pkey -in "$WORK/AuthKey.p8" -noout 2>/dev/null || {
  printf 'APPLE_API_KEY_P8 is not the base64 of the App Store Connect .p8 key\n' >&2
  exit 1
}

printf 'APPLE_API_KEY_PATH=%s\nMACOS_DEV_IDENTITY=%s\n' "$WORK/AuthKey.p8" "$IDENTITY" >&3
