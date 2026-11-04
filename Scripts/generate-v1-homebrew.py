#!/usr/bin/env python3
"""Generate a local stable-release cask from the final macOS DMGs."""
import hashlib
import json
import pathlib
import re
import sys


def generate(directory, config):
    version = config["version"]
    if not re.fullmatch(r"(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)", version):
        raise ValueError("Homebrew candidate requires a stable version")
    if config["identifier"] != "com.quosparr.app":
        raise ValueError("Homebrew candidate requires the production bundle identity")
    checksums = {}
    for arch in ("arm64", "x86_64"):
        path = directory / f"Quosparr-macos-{arch}.dmg"
        if path.stat().st_size == 0:
            raise ValueError("final DMG must not be empty")
        digest = hashlib.sha256()
        with path.open("rb") as file:
            for chunk in iter(lambda: file.read(1024 * 1024), b""):
                digest.update(chunk)
        checksums[arch] = digest.hexdigest()
    return f'''cask "quosparr" do
  arch arm: "arm64", intel: "x86_64"
  version "{version}"
  sha256 arm:   "{checksums['arm64']}",
         intel: "{checksums['x86_64']}"
  url "https://github.com/Licoy/Quosparr/releases/download/v#{{version}}/Quosparr-macos-#{{arch}}.dmg",
      verified: "github.com/Licoy/Quosparr/"
  name "Quosparr"
  desc "Native menu bar monitor for quota, usage, and managed accounts"
  homepage "https://github.com/Licoy/Quosparr"
  livecheck do
    url :url
    strategy :github_latest
  end
  auto_updates true
  depends_on macos: :monterey
  app "Quosparr.app"
  uninstall quit: "com.quosparr.app"
  zap trash: [
    "~/.quosparr",
    "~/Library/Application Support/Quosparr",
    "~/Library/Preferences/com.quosparr.app.plist",
  ]
  caveats <<~EOS
    Use brew upgrade --cask --greedy quosparr for Homebrew upgrades
  EOS
end
'''


def main():
    directory, config_path, output = map(pathlib.Path, sys.argv[1:4])
    candidate = generate(directory, json.loads(config_path.read_text()))
    output.write_text(candidate)
    print(f"Local Homebrew candidate generated: {output}; remote tap unchanged")


if __name__ == "__main__":
    main()
