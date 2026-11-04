#!/usr/bin/env bash
# Prints the per-OS download section appended to the GitHub release notes.
# Links are built from the final asset names so beta and Debian version
# spellings never drift; a missing asset fails instead of publishing a dead link.
set -euo pipefail

if [[ $# -ne 3 ]]; then
  echo "usage: $0 <dist-dir> <tag> <owner/repo>" >&2
  exit 2
fi
dist="$1"
base="https://github.com/$3/releases/download/$2"

# link <label> <glob>: markdown link to the single asset matching glob
link() {
  local matches=()
  local path
  for path in "$dist"/$2; do
    [[ -e "$path" ]] && matches+=("${path##*/}")
  done
  if [[ ${#matches[@]} -ne 1 ]]; then
    echo "expected one asset matching $2, found ${#matches[@]}" >&2
    return 1
  fi
  printf '[%s](%s/%s)' "$1" "$base" "${matches[0]}"
}

# One assignment per link: in "$(a) · $(b)" only b's status reaches set -e
mac_arm_dmg="$(link DMG 'Quosparr-macos-arm64.dmg')"
mac_arm_zip="$(link ZIP 'Quosparr-macos-arm64.zip')"
mac_x64_dmg="$(link DMG 'Quosparr-macos-x86_64.dmg')"
mac_x64_zip="$(link ZIP 'Quosparr-macos-x86_64.zip')"
win_x64_setup="$(link Installer 'Quosparr*Setup*amd64.exe')"
win_x64_tar="$(link 'Portable (.tar.gz)' 'quosparr-*-windows-amd64.tar.gz')"
win_arm_setup="$(link Installer 'Quosparr*Setup*arm64.exe')"
win_arm_tar="$(link 'Portable (.tar.gz)' 'quosparr-*-windows-arm64.tar.gz')"
linux_x64_deb="$(link .deb 'quosparr_*_amd64.deb')"
linux_x64_tar="$(link .tar.gz 'quosparr-*-linux-amd64.tar.gz')"
linux_arm_deb="$(link .deb 'quosparr_*_arm64.deb')"
linux_arm_tar="$(link .tar.gz 'quosparr-*-linux-arm64.tar.gz')"
install_sh="$(link install.sh 'install.sh')"
checksums="$(link SHA256SUMS 'SHA256SUMS')"

cat <<EOF

---

## Downloads

### macOS

macOS 12 or later. Desktop widgets are included and require macOS 14 or later.

| Architecture | Download |
| --- | --- |
| Apple Silicon (M series) | $mac_arm_dmg · $mac_arm_zip |
| Intel | $mac_x64_dmg · $mac_x64_zip |

### Windows

Windows 10 or 11.

| Architecture | Download |
| --- | --- |
| x64 | $win_x64_setup · $win_x64_tar |
| ARM64 | $win_arm_setup · $win_arm_tar |

The installer installs for the current user into \`%LOCALAPPDATA%\\Programs\\Quosparr\` and supports in-app updates; no administrator rights needed. The portable build needs no installation: extract it and run \`Quosparr.exe\` from any folder you can write to.

### Linux

Needs GTK3, AppIndicator, a desktop with a system tray, and a Secret Service session such as GNOME Keyring or KWallet.

| Architecture | Download |
| --- | --- |
| x64 (amd64) | $linux_x64_deb · $linux_x64_tar |
| ARM64 | $linux_arm_deb · $linux_arm_tar |

Install the .deb with your package manager, or put $install_sh next to the .tar.gz and run \`sh install.sh\` to install into \`~/.local\`.

Verify downloads with $checksums.
EOF
